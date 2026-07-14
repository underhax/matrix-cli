// Package main is the entry point for the matrix-cli application.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/underhax/matrix-cli/internal/client"
	"github.com/underhax/matrix-cli/internal/config"
	"github.com/underhax/matrix-cli/internal/consts"
	"github.com/underhax/matrix-cli/internal/logger"
	"github.com/underhax/matrix-cli/internal/store"
)

var (
	AppVersion            = "dev"
	osExit                = os.Exit
	runtimeGOOS           = runtime.GOOS
	filepathAbs           = filepath.Abs
	stdout      io.Writer = os.Stdout
	dbClose               = (*sql.DB).Close
	osRemove              = os.Remove
)

const (
	modeAuth      = "auth"
	modeBootstrap = "bootstrap"
	modeListen    = "listen"
	modeSend      = "send"
	modeVerify    = "verify"
	modeRooms     = "rooms"
	modeRoomInfo  = "room-info"
	modeDevices   = "devices"
	modeLogout    = "logout"

	flagMode            = "--mode"
	flagServer          = "--server"
	flagUser            = "--user"
	flagPass            = "--pass"
	flagNewKeys         = "--new-keys"
	flagDevice          = "--device"
	flagSSOCallbackPort = "--sso-callback-port"
	flagRecoveryKey     = "--recovery-key"
	flagRooms           = "--rooms"
	flagMessage         = "--message"
	flagVerbose         = "--verbose"
	flagDebug           = "--debug"
	flagDataDir         = "--data-dir"
)

func getDefaultDataDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "matrix-cli")
	}
	if home := os.Getenv("HOME"); home != "" {
		if runtimeGOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support", "matrix-cli")
		}
		return filepath.Join(home, ".config", "matrix-cli")
	}
	if appData := os.Getenv("AppData"); appData != "" {
		return filepath.Join(appData, "matrix-cli")
	}
	return "."
}

type cliOptions struct {
	mode            *string
	server          *string
	user            *string
	pass            *string
	newKeys         *bool
	device          *string
	ssoCallbackPort *string
	recoveryKey     *string
	rooms           *string
	msg             *string
	verbose         *bool
	debugLevel      *int
	dataDir         *string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
}

func setupFlags() (*flag.FlagSet, cliOptions) {
	fs := flag.NewFlagSet("matrix-cli", flag.ContinueOnError)
	mode := fs.String(flagMode[2:], "", "Execution mode: auth, bootstrap, listen, send, verify, rooms, room-info, devices")
	server := fs.String(flagServer[2:], "https://matrix.org", "Homeserver URL (for auth)")
	user := fs.String(flagUser[2:], "", "Matrix user ID (for auth and verify)")
	pass := fs.String(flagPass[2:], "", "Matrix password (for auth)")
	newKeys := fs.Bool(flagNewKeys[2:], false, "Generate new SSSS and cross-signing keys (for bootstrap)")
	device := fs.String(flagDevice[2:], "", "Device display name (for auth)")
	ssoCallbackPort := fs.String(flagSSOCallbackPort[2:], "", "Force a specific port for SSO callback (e.g. 8080) (for auth)")
	recoveryKey := fs.String(flagRecoveryKey[2:], "", "Recovery key for SSSS (for bootstrap)")
	rooms := fs.String(flagRooms[2:], "", "Target room ID(s) (space-separated for send, room-info, listen)")
	msg := fs.String(flagMessage[2:], "", "Message body (for send)")
	verbose := fs.Bool(flagVerbose[2:], false, "Enable verbose output (e.g. detailed room info)")
	debugLevel := 0
	fs.Var(&logger.LevelFlag{Level: &debugLevel}, flagDebug[2:], "Enable debug logging (use --debug or --debug=2)")

	defaultDataDir := getDefaultDataDir()
	dataDir := fs.String(flagDataDir[2:], defaultDataDir, "Directory to store session and database files")

	fs.Usage = func() {
		modeVal := *mode
		if modeVal == "" {
			for i, arg := range os.Args {
				if arg == flagMode && i+1 < len(os.Args) {
					modeVal = os.Args[i+1]
					break
				}
			}
		}
		printUsage(modeVal)
	}

	opts := cliOptions{
		mode:            mode,
		server:          server,
		user:            user,
		pass:            pass,
		newKeys:         newKeys,
		device:          device,
		ssoCallbackPort: ssoCallbackPort,
		recoveryKey:     recoveryKey,
		rooms:           rooms,
		msg:             msg,
		verbose:         verbose,
		debugLevel:      &debugLevel,
		dataDir:         dataDir,
	}

	return fs, opts
}

func run(args []string) error {
	fs, opts := setupFlags()

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	log := logger.Setup(*opts.debugLevel, os.Stderr)

	if *opts.mode == "" || *opts.mode == "-h" || *opts.mode == "help" || *opts.mode == "--help" {
		fs.Usage()
		return nil
	}

	validModes := map[string]bool{
		modeAuth: true, modeBootstrap: true, modeListen: true, modeSend: true,
		modeVerify: true, modeRooms: true, modeRoomInfo: true,
		modeDevices: true, modeLogout: true,
	}
	if !validModes[*opts.mode] {
		fs.Usage()
		return fmt.Errorf("unknown mode: %s", *opts.mode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *opts.debugLevel >= 2 {
		ctx = log.WithContext(ctx)
	}

	if err := os.MkdirAll(*opts.dataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	sessionFile := filepath.Join(*opts.dataDir, "session.json")
	dbFile := filepath.Join(*opts.dataDir, "crypto.db")
	pickleFile := filepath.Join(*opts.dataDir, "pickle.key")

	if *opts.mode == modeAuth {
		return handleAuth(ctx, *opts.server, *opts.user, *opts.pass, *opts.device, *opts.ssoCallbackPort, sessionFile)
	}

	return handleOperations(ctx, &log, *opts.mode, *opts.rooms, *opts.msg, *opts.user, *opts.newKeys, *opts.recoveryKey, *opts.verbose, sessionFile, dbFile, pickleFile)
}

func handleAuth(ctx context.Context, server, user, pass, device, ssoCallbackPort, sessionFile string) error {
	session, err := client.Login(ctx, server, user, pass, device, ssoCallbackPort)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := config.Save(sessionFile, session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	absPath, errAbs := filepathAbs(sessionFile)
	if errAbs != nil || absPath == "" {
		absPath = sessionFile
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nAuthentication successful. Session saved to %s\n", absPath)

	out := map[string]string{
		consts.KeyStatus:     "success",
		consts.KeyUserID:     session.UserID,
		consts.KeyDeviceID:   session.DeviceID,
		consts.KeyDeviceName: session.DeviceName,
	}
	if payload, marshalErr := json.Marshal(out); marshalErr == nil {
		if _, writeErr := fmt.Fprintf(stdout, "\n%s\n\n", string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}

	return nil
}

func handleOperations(ctx context.Context, log *logger.Logger, mode, rooms, msg, targetUser string, newKeys bool, recoveryKey string, verbose bool, sessionFile, dbFile, pickleFile string) error {
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
			if closeErr := dbClose(db); closeErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "failed to close database: %v\n", closeErr)
			}
		}
	}()

	if mode == modeLogout {
		handleLogout(ctx, session, db, &dbClosed, sessionFile, dbFile, pickleFile)
		return nil
	}

	cli, err := client.New(ctx, session, db, pickleFile, log)
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
		if closeErr := dbClose(db); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to close db: %v\n", closeErr)
		}
		*dbClosed = true
	}

	for _, f := range []string{sessionFile, dbFile, dbFile + "-wal", dbFile + "-shm", pickleFile} {
		if rmErr := osRemove(f); rmErr != nil && !os.IsNotExist(rmErr) {
			_, _ = fmt.Fprintf(os.Stderr, "failed to remove %s: %v\n", f, rmErr)
		}
	}

	out := map[string]string{
		"status": "success",
	}
	if payload, marshalErr := json.Marshal(out); marshalErr == nil {
		if _, writeErr := fmt.Fprintf(stdout, "\n%s\n\n", string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}
}

func executeMode(ctx context.Context, cli *client.Client, mode, rooms, msg, targetUser string, newKeys bool, recoveryKey string, verbose bool) error {
	switch mode {
	case modeBootstrap:
		if err := cli.Bootstrap(ctx, newKeys, recoveryKey); err != nil {
			return fmt.Errorf("bootstrap error: %w", err)
		}
	case modeListen:
		if err := cli.Listen(ctx, rooms); err != nil {
			return fmt.Errorf("listener error: %w", err)
		}
	case modeSend:
		if rooms == "" || msg == "" {
			return errors.New("--rooms and --message are required for send mode")
		}
		if err := cli.Send(ctx, rooms, msg); err != nil {
			return fmt.Errorf("send error: %w", err)
		}
	case modeVerify:
		if err := cli.Verify(ctx, targetUser); err != nil {
			return fmt.Errorf("verify mode error: %w", err)
		}
	case modeRooms, modeRoomInfo:
		return executeRoomsInfo(ctx, cli, mode, rooms, verbose)
	case modeDevices:
		if err := cli.Devices(ctx); err != nil {
			return fmt.Errorf("devices fetch error: %w", err)
		}
	default:
		return errors.New("unknown or missing --mode. Allowed: auth, bootstrap, listen, send, verify, rooms, room-info, devices, logout")
	}

	return nil
}

func executeRoomsInfo(ctx context.Context, cli *client.Client, mode, rooms string, verbose bool) error {
	if mode == modeRooms {
		if _, err := fmt.Fprintf(stdout, "\nJoined Rooms:\n"); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
		if err := cli.Rooms(ctx, verbose); err != nil {
			return fmt.Errorf("rooms list error: %w", err)
		}
		return nil
	}

	if rooms == "" {
		return errors.New("--rooms is required for room-info mode")
	}
	if err := cli.RoomInfo(ctx, rooms); err != nil {
		return fmt.Errorf("room info error: %w", err)
	}
	return nil
}
