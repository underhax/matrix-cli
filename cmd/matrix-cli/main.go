// Package main is the entry point for the matrix-cli application.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"matrix-cli/internal/client"
	"matrix-cli/internal/config"
	"matrix-cli/internal/store"
)

const (
	ModeAuth     = "auth"
	ModeListen   = "listen"
	ModeSend     = "send"
	ModeVerify   = "verify"
	ModeRooms    = "rooms"
	ModeRoomInfo = "room-info"
	ModeDevices  = "devices"
	ModeLogout   = "logout"
)

func getDefaultDataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return filepath.Join(dir, "matrix-cli")
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "Execution mode: auth, listen, send, verify, rooms, room-info, devices")
	server := flag.String("server", "https://matrix.org", "Homeserver URL (for auth)")
	user := flag.String("user", "", "Matrix user ID (for auth)")
	pass := flag.String("pass", "", "Matrix password (for auth)")
	device := flag.String("device", "", "Device display name (for auth)")
	rooms := flag.String("rooms", "", "Target room ID(s) (space-separated for send, room-info, listen)")
	msg := flag.String("message", "", "Message body (for send)")
	verbose := flag.Bool("verbose", false, "Enable verbose output (e.g. detailed room info)")

	defaultDataDir := getDefaultDataDir()
	dataDir := flag.String("data-dir", defaultDataDir, "Directory to store session and database files")

	flag.Usage = func() {
		modeVal := *mode
		if modeVal == "" {
			for i, arg := range os.Args {
				if arg == "--mode" && i+1 < len(os.Args) {
					modeVal = os.Args[i+1]
					break
				}
			}
		}
		printUsage(modeVal)
	}

	flag.Parse()

	if *mode == "" || *mode == "-h" || *mode == "help" || *mode == "--help" {
		flag.Usage()
		return nil
	}

	validModes := map[string]bool{
		ModeAuth: true, ModeListen: true, ModeSend: true,
		ModeVerify: true, ModeRooms: true, ModeRoomInfo: true,
		ModeDevices: true, ModeLogout: true,
	}
	if !validModes[*mode] {
		flag.Usage()
		return fmt.Errorf("unknown mode: %s", *mode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	sessionFile := filepath.Join(*dataDir, "session.json")
	dbFile := filepath.Join(*dataDir, "crypto.db")

	if *mode == ModeAuth {
		return handleAuth(ctx, *server, *user, *pass, *device, sessionFile)
	}

	return handleOperations(ctx, *mode, *rooms, *msg, *verbose, sessionFile, dbFile)
}

func handleAuth(ctx context.Context, server, user, pass, device, sessionFile string) error {
	if user == "" || pass == "" {
		return errors.New("--user and --pass are required for auth mode")
	}

	session, err := client.Login(ctx, server, user, pass, device)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := config.Save(sessionFile, session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	absPath, errAbs := filepath.Abs(sessionFile)
	if errAbs != nil || absPath == "" {
		absPath = sessionFile
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nAuthentication successful. Session saved to %s\n", absPath)

	out := map[string]string{
		"status":      "success",
		"user_id":     session.UserID,
		"device_id":   session.DeviceID,
		"device_name": session.DeviceName,
	}
	if payload, marshalErr := json.Marshal(out); marshalErr == nil {
		if _, writeErr := fmt.Fprintf(os.Stdout, "\n%s\n\n", string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}

	return nil
}

func handleOperations(ctx context.Context, mode, rooms, msg string, verbose bool, sessionFile, dbFile string) error {
	session, err := config.Load(sessionFile)
	if err != nil {
		return fmt.Errorf("failed to load session (run --mode auth first): %w", err)
	}

	db, err := store.OpenDB(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	dbClosed := false
	defer func() {
		if !dbClosed {
			if closeErr := db.Close(); closeErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "failed to close database: %v\n", closeErr)
			}
		}
	}()

	if mode == ModeLogout {
		handleLogout(ctx, session, db, &dbClosed, sessionFile, dbFile)
		return nil
	}

	cli, err := client.New(ctx, session, db)
	if err != nil {
		return fmt.Errorf("client initialization failed: %w", err)
	}

	return executeMode(ctx, cli, mode, rooms, msg, verbose)
}

func handleLogout(ctx context.Context, session *config.Session, db *sql.DB, dbClosed *bool, sessionFile, dbFile string) {
	if err := client.LogoutSession(ctx, session); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: server logout failed (local data will still be wiped): %v\n", err)
	}

	if !*dbClosed {
		if closeErr := db.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to close db: %v\n", closeErr)
		}
		*dbClosed = true
	}

	for _, f := range []string{sessionFile, dbFile, dbFile + "-wal", dbFile + "-shm"} {
		if rmErr := os.Remove(f); rmErr != nil && !os.IsNotExist(rmErr) {
			_, _ = fmt.Fprintf(os.Stderr, "failed to remove %s: %v\n", f, rmErr)
		}
	}

	out := map[string]string{
		"status": "success",
	}
	if payload, marshalErr := json.Marshal(out); marshalErr == nil {
		if _, writeErr := fmt.Fprintf(os.Stdout, "\n%s\n\n", string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}
}

func executeMode(ctx context.Context, cli *client.Client, mode, rooms, msg string, verbose bool) error {
	switch mode {
	case ModeListen:
		if err := cli.Listen(ctx, rooms); err != nil {
			return fmt.Errorf("listener error: %w", err)
		}
	case ModeSend:
		if rooms == "" || msg == "" {
			return errors.New("--rooms and --message are required for send mode")
		}
		if err := cli.Send(ctx, rooms, msg); err != nil {
			return fmt.Errorf("send error: %w", err)
		}
	case ModeVerify:
		if err := cli.Verify(ctx); err != nil {
			return fmt.Errorf("verify mode error: %w", err)
		}
	case ModeRooms, ModeRoomInfo:
		return executeRoomsInfo(ctx, cli, mode, rooms, verbose)
	case ModeDevices:
		if err := cli.Devices(ctx); err != nil {
			return fmt.Errorf("devices fetch error: %w", err)
		}
	default:
		return errors.New("unknown or missing --mode. Allowed: auth, listen, send, verify, rooms, room-info, devices, logout")
	}

	return nil
}

func executeRoomsInfo(ctx context.Context, cli *client.Client, mode, rooms string, verbose bool) error {
	if mode == ModeRooms {
		if err := cli.Rooms(ctx, verbose); err != nil {
			return fmt.Errorf("rooms list error: %w", err)
		}
	} else {
		if rooms == "" {
			return errors.New("--rooms is required for room-info mode")
		}
		if err := cli.RoomInfo(ctx, rooms); err != nil {
			return fmt.Errorf("room info error: %w", err)
		}
	}
	return nil
}

func printUsage(modeVal string) {
	switch modeVal {
	case ModeAuth:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode auth --server <DOMAIN_OR_URL> --user <ID> --pass <PASSWORD> [--device <NAME>] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Login to Matrix and save session.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Auto-discover API URL via .well-known (recommended):\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'matrix.org' --user '@bot:matrix.org' --pass 's3cret'\n\n")
		fmt.Fprintf(os.Stderr, "  # Specify exact HTTPS URL:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'https://synapse.example.com' --user '@bot:example.com' --pass 's3cret'\n\n")
		fmt.Fprintf(os.Stderr, "  # Specify local HTTP URL with port and custom device name:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode auth --server 'http://127.0.0.1:8008' --user '@bot:localhost' --pass 's3cret' --device 'MyBot'\n")
	case ModeListen:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode listen [--rooms \"<ID1> <ID2>\"] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Listen for incoming messages and events. If --rooms is provided, only events from those rooms are processed.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode listen --rooms \"!room1:example.com !room2:example.com\"\n")
	case ModeSend:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode send --rooms \"<ID>\" --message \"<TEXT>\" [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Send a message to one or more rooms.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode send --rooms \"!room1:example.com\" --message \"Hello world!\"\n")
	case ModeVerify:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode verify [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Start an interactive device verification (SAS) flow.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode verify\n")
	case ModeRooms:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode rooms [--verbose] [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "List joined rooms.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode rooms\n")
	case ModeRoomInfo:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode room-info --rooms \"<ID>\" [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Get detailed info for specific room(s).\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode room-info --rooms \"!room1:example.com\"\n")
	case ModeDevices:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode devices [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "List active devices for the account.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode devices\n")
	case ModeLogout:
		fmt.Fprintf(os.Stderr, "Usage: matrix-cli --mode logout [--data-dir <PATH>]\n")
		fmt.Fprintf(os.Stderr, "Logout from the homeserver and delete the local session and database.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  matrix-cli --mode logout\n")
	default:
		printGlobalUsage()
		return
	}
	printUsageFooter(modeVal)
}

func printGlobalUsage() {
	fmt.Fprintf(os.Stderr, "matrix-cli - A headless Matrix client\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  matrix-cli --mode <mode> [options]\n\n")
	fmt.Fprintf(os.Stderr, "Modes:\n")
	printModeList("")
	fmt.Fprintf(os.Stderr, "Tip: Run 'matrix-cli --mode <mode> -h' for mode-specific help.\n\n")
	fmt.Fprintf(os.Stderr, "Global Options:\n")
	fmt.Fprintf(os.Stderr, "  -data-dir string\n")
	fmt.Fprintf(os.Stderr, "        Directory to store session and database files (default %q)\n", getDefaultDataDir())
}

func printUsageFooter(exclude string) {
	fmt.Fprintf(os.Stderr, "\nOther modes:\n")
	printModeList(exclude)
	fmt.Fprintf(os.Stderr, "Global Options:\n")
	fmt.Fprintf(os.Stderr, "  -data-dir string\n")
	fmt.Fprintf(os.Stderr, "        Directory to store session and database files (default %q)\n", getDefaultDataDir())
}

func printModeList(exclude string) {
	modes := []struct {
		name string
		desc string
	}{
		{ModeAuth, "Login to Matrix and save session"},
		{ModeListen, "Listen for incoming messages and events"},
		{ModeSend, "Send a message to a room"},
		{ModeVerify, "Start an interactive device verification (SAS) flow"},
		{ModeRooms, "List joined rooms"},
		{ModeRoomInfo, "Get detailed info for a specific room"},
		{ModeDevices, "List active devices for the account"},
		{ModeLogout, "Logout and clear local session"},
	}
	for _, m := range modes {
		if m.name != exclude {
			fmt.Fprintf(os.Stderr, "  %-10s %s\n", m.name, m.desc)
		}
	}
	fmt.Fprintln(os.Stderr)
}
