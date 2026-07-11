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

	"github.com/rs/zerolog"
)

const (
	ModeAuth      = "auth"
	ModeBootstrap = "bootstrap"
	ModeListen    = "listen"
	ModeSend      = "send"
	ModeVerify    = "verify"
	ModeRooms     = "rooms"
	ModeRoomInfo  = "room-info"
	ModeDevices   = "devices"
	ModeLogout    = "logout"
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
	mode := flag.String("mode", "", "Execution mode: auth, bootstrap, listen, send, verify, rooms, room-info, devices")
	server := flag.String("server", "https://matrix.org", "Homeserver URL (for auth)")
	user := flag.String("user", "", "Matrix user ID (for auth and verify)")
	pass := flag.String("pass", "", "Matrix password (for auth)")
	newKeys := flag.Bool("new-keys", false, "Generate new SSSS and cross-signing keys (for bootstrap)")
	device := flag.String("device", "", "Device display name (for auth)")
	ssoCallbackPort := flag.String("sso-callback-port", "", "Force a specific port for SSO callback (e.g. 8080) (for auth)")
	recoveryKey := flag.String("recovery-key", "", "Recovery key for SSSS (for bootstrap)")
	rooms := flag.String("rooms", "", "Target room ID(s) (space-separated for send, room-info, listen)")
	msg := flag.String("message", "", "Message body (for send)")
	verbose := flag.Bool("verbose", false, "Enable verbose output (e.g. detailed room info)")
	debugFlag := flag.Bool("debug", false, "Enable debug logging for secrets and hooks")

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

	if *debugFlag {
		client.DebugMode = true
	}

	if *mode == "" || *mode == "-h" || *mode == "help" || *mode == "--help" {
		flag.Usage()
		return nil
	}

	validModes := map[string]bool{
		ModeAuth: true, ModeBootstrap: true, ModeListen: true, ModeSend: true,
		ModeVerify: true, ModeRooms: true, ModeRoomInfo: true,
		ModeDevices: true, ModeLogout: true,
	}
	if !validModes[*mode] {
		flag.Usage()
		return fmt.Errorf("unknown mode: %s", *mode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *verbose {
		logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger().Level(zerolog.DebugLevel)
		ctx = logger.WithContext(ctx)
	}

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	sessionFile := filepath.Join(*dataDir, "session.json")
	dbFile := filepath.Join(*dataDir, "crypto.db")
	pickleFile := filepath.Join(*dataDir, "pickle.key")

	if *mode == ModeAuth {
		return handleAuth(ctx, *server, *user, *pass, *device, *ssoCallbackPort, sessionFile)
	}

	return handleOperations(ctx, *mode, *rooms, *msg, *user, *newKeys, *recoveryKey, *verbose, sessionFile, dbFile, pickleFile)
}

func handleAuth(ctx context.Context, server, user, pass, device, ssoCallbackPort, sessionFile string) error {
	session, err := client.Login(ctx, server, user, pass, device, ssoCallbackPort)
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

func handleOperations(ctx context.Context, mode, rooms, msg, targetUser string, newKeys bool, recoveryKey string, verbose bool, sessionFile, dbFile, pickleFile string) error {
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
		handleLogout(ctx, session, db, &dbClosed, sessionFile, dbFile, pickleFile)
		return nil
	}

	cli, err := client.New(ctx, session, db, pickleFile)
	if err != nil {
		return fmt.Errorf("client initialization failed: %w", err)
	}

	return executeMode(ctx, cli, mode, rooms, msg, targetUser, newKeys, recoveryKey, verbose)
}

func handleLogout(ctx context.Context, session *config.Session, db *sql.DB, dbClosed *bool, sessionFile, dbFile, pickleFile string) {
	if err := client.LogoutSession(ctx, session); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: server logout failed (local data will still be wiped): %v\n", err)
	}

	if !*dbClosed {
		if closeErr := db.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to close db: %v\n", closeErr)
		}
		*dbClosed = true
	}

	for _, f := range []string{sessionFile, dbFile, dbFile + "-wal", dbFile + "-shm", pickleFile} {
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

func executeMode(ctx context.Context, cli *client.Client, mode, rooms, msg, targetUser string, newKeys bool, recoveryKey string, verbose bool) error {
	switch mode {
	case ModeBootstrap:
		if err := cli.Bootstrap(ctx, newKeys, recoveryKey); err != nil {
			return fmt.Errorf("bootstrap error: %w", err)
		}
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
		if err := cli.Verify(ctx, targetUser); err != nil {
			return fmt.Errorf("verify mode error: %w", err)
		}
	case ModeRooms, ModeRoomInfo:
		return executeRoomsInfo(ctx, cli, mode, rooms, verbose)
	case ModeDevices:
		if err := cli.Devices(ctx); err != nil {
			return fmt.Errorf("devices fetch error: %w", err)
		}
	default:
		return errors.New("unknown or missing --mode. Allowed: auth, bootstrap, listen, send, verify, rooms, room-info, devices, logout")
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
