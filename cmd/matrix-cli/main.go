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
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/underhax/matrix-cli/internal/client"
	"github.com/underhax/matrix-cli/internal/config"
	"github.com/underhax/matrix-cli/internal/consts"
	"github.com/underhax/matrix-cli/internal/logger"
	"github.com/underhax/matrix-cli/internal/store"
	"github.com/underhax/matrix-cli/internal/updater"
	"github.com/underhax/matrix-cli/internal/validator"
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
	cmdUpdate  = "update"
	cmdVersion = "version"

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
	flagHTML            = "--html"
	flagMarkdown        = "--markdown"
	flagVerbose         = "--verbose"
	flagDebug           = "--debug"
	flagDataDir         = "--data-dir"
	flagJSON            = "--json"
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
	html            *bool
	markdown        *bool
	verbose         *bool
	debugLevel      *int
	dataDir         *string
	jsonMode        *bool
}

func isJSONMode() bool {
	return slices.Contains(os.Args[1:], flagJSON)
}

var jsonMarshal = json.Marshal

func printFatalError(err error) {
	if isJSONMode() {
		errMap := map[string]string{
			"level": "fatal",
			"error": err.Error(),
			"time":  time.Now().Format(time.RFC3339),
		}
		if payload, mErr := jsonMarshal(errMap); mErr == nil {
			fmt.Fprintln(os.Stderr, string(payload))
		} else {
			fmt.Fprintf(os.Stderr, `{"level":"fatal","error":"%s"}`+"\n", err.Error())
		}
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func main() {
	setUmask()
	updater.CleanupWindowsOldFiles()
	if err := run(os.Args[1:]); err != nil {
		printFatalError(err)
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
	html := fs.Bool(flagHTML[2:], false, "Send message as HTML formatted text (for send)")
	markdown := fs.Bool(flagMarkdown[2:], false, "Parse message as Markdown and send formatted text (for send)")
	verbose := fs.Bool(flagVerbose[2:], false, "Enable verbose output (e.g. detailed room info)")
	jsonMode := fs.Bool(flagJSON[2:], false, "Enable strict machine mode (JSON output only, no interactive prompts)")
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
		html:            html,
		markdown:        markdown,
		verbose:         verbose,
		debugLevel:      &debugLevel,
		dataDir:         dataDir,
		jsonMode:        jsonMode,
	}

	return fs, opts
}

func checkCommands(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case cmdUpdate:
		return true, handleUpdate(context.Background())
	case cmdVersion:
		fmt.Fprintln(os.Stderr, AppVersion)
		return true, nil
	}
	return false, nil
}

func checkModeOpts(mode string, jsonMode bool) (bool, error) {
	if mode == "" || mode == "-h" || mode == "help" || mode == "--help" {
		return true, nil
	}

	validModes := map[string]bool{
		consts.ModeAuth: true, consts.ModeBootstrap: true, consts.ModeListen: true, consts.ModeSend: true,
		consts.ModeVerify: true, consts.ModeRooms: true, consts.ModeRoomInfo: true,
		consts.ModeDevices: true, consts.ModeLogout: true,
	}
	if !validModes[mode] {
		return false, fmt.Errorf("unknown mode: %s", mode)
	}

	if jsonMode && (mode == consts.ModeVerify || mode == consts.ModeBootstrap || mode == consts.ModeAuth || mode == consts.ModeLogout) {
		return false, fmt.Errorf("--json flag is not supported in %s mode", mode)
	}

	return false, nil
}

func run(args []string) error {
	if handled, err := checkCommands(args); handled {
		return err
	}

	fs, opts := setupFlags()

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	log := logger.Setup(*opts.debugLevel, *opts.jsonMode, os.Stderr)

	client.InteractiveDisabled = *opts.jsonMode
	client.JSONMode = *opts.jsonMode

	help, err := checkModeOpts(*opts.mode, *opts.jsonMode)
	if err != nil {
		if !*opts.jsonMode {
			fs.Usage()
		}
		return err
	}
	if help {
		fs.Usage()
		return nil
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

	if msgs := validateInput(*opts.mode, *opts.server, *opts.user, *opts.rooms, sessionFile, dbFile, pickleFile); len(msgs) > 0 {
		for _, msg := range msgs {
			_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		}
		return nil
	}

	if *opts.mode == consts.ModeAuth {
		return handleAuth(ctx, *opts.server, *opts.user, *opts.pass, *opts.device, *opts.ssoCallbackPort, sessionFile)
	}

	return handleOperations(ctx, &log, *opts.mode, *opts.rooms, *opts.msg, *opts.user, *opts.newKeys, *opts.recoveryKey, *opts.verbose, *opts.html, *opts.markdown, sessionFile, dbFile, pickleFile)
}

func validateInput(mode, server, user, rooms, sessionFile, dbFile, pickleFile string) []string {
	msgs := make([]string, 0, 8)

	msgs = append(msgs, validateAuthInput(mode, server, user)...)
	msgs = append(msgs, validateRoomsInput(mode, rooms)...)
	msgs = append(msgs, validateFilesPerms(sessionFile, dbFile, pickleFile)...)

	return msgs
}

func validateAuthInput(mode, server, user string) []string {
	var msgs []string
	if mode == consts.ModeAuth {
		if err := validateServerURL(server); err != nil {
			msgs = append(msgs, fmt.Sprintf("%v for server %q", err, server))
		}
	}
	if user != "" && (mode == consts.ModeAuth || mode == consts.ModeVerify) {
		if err := validator.ValidateUserID(user); err != nil {
			msgs = append(msgs, fmt.Sprintf("%v for user %q", err, user))
		}
	}
	return msgs
}

func validateServerURL(server string) error {
	if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
		if err := validator.ValidateURL(server); err != nil {
			return fmt.Errorf("%w", err)
		}
		return nil
	}
	if err := validator.ValidateDomain(server); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func validateRoomsInput(mode, rooms string) []string {
	var msgs []string
	if rooms != "" && (mode == consts.ModeSend || mode == consts.ModeListen || mode == consts.ModeRoomInfo) {
		for r := range strings.FieldsSeq(rooms) {
			if err := validator.ValidateRoomID(r); err != nil {
				msgs = append(msgs, fmt.Sprintf("%v for room %q", err, r))
			}
		}
	}
	return msgs
}

func validateFilesPerms(files ...string) []string {
	var msgs []string
	for _, file := range files {
		if err := validator.ValidatePermissions(file); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				msgs = append(msgs, fmt.Sprintf("insecure permissions on %q (expected 0600)", file))
			}
		}
	}
	return msgs
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

	_, _ = fmt.Fprintf(os.Stderr, "\nAuthentication successful.\n")
	_, _ = fmt.Fprintf(os.Stderr, "  User ID:     %s\n", session.UserID)
	_, _ = fmt.Fprintf(os.Stderr, "  Device ID:   %s\n", session.DeviceID)
	if session.DeviceName != "" {
		_, _ = fmt.Fprintf(os.Stderr, "  Device Name: %s\n", session.DeviceName)
	}
	_, _ = fmt.Fprintf(os.Stderr, "\nSession saved to %s\n", absPath)

	return nil
}

func handleOperations(ctx context.Context, log *logger.Logger, mode, rooms, msg, targetUser string, newKeys bool, recoveryKey string, verbose, html, markdown bool, sessionFile, dbFile, pickleFile string) error {
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

	if mode == consts.ModeLogout {
		handleLogout(ctx, session, db, &dbClosed, sessionFile, dbFile, pickleFile)
		return nil
	}

	cli, err := client.New(ctx, session, db, pickleFile, log, mode)
	if err != nil {
		return fmt.Errorf("client initialization failed: %w", err)
	}

	return executeMode(ctx, cli, mode, rooms, msg, targetUser, newKeys, recoveryKey, verbose, html, markdown)
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

	_, _ = fmt.Fprintf(os.Stderr, "\nLogout successful. Local data wiped.\n")
}

func executeMode(ctx context.Context, cli *client.Client, mode, rooms, msg, targetUser string, newKeys bool, recoveryKey string, verbose, html, markdown bool) error {
	switch mode {
	case consts.ModeBootstrap:
		if err := cli.Bootstrap(ctx, newKeys, recoveryKey); err != nil {
			return fmt.Errorf("bootstrap error: %w", err)
		}
	case consts.ModeListen:
		if err := cli.Listen(ctx, rooms); err != nil {
			return fmt.Errorf("listener error: %w", err)
		}
	case consts.ModeSend:
		if rooms == "" || msg == "" {
			return errors.New("--rooms and --message are required for send mode")
		}
		if err := cli.Send(ctx, rooms, msg, html, markdown); err != nil {
			return fmt.Errorf("send error: %w", err)
		}
	case consts.ModeVerify:
		if err := cli.Verify(ctx, targetUser); err != nil {
			return fmt.Errorf("verify mode error: %w", err)
		}
	case consts.ModeRooms, consts.ModeRoomInfo:
		return executeRoomsInfo(ctx, cli, mode, rooms, verbose)
	case consts.ModeDevices:
		if err := cli.Devices(ctx); err != nil {
			return fmt.Errorf("devices fetch error: %w", err)
		}
	default:
		return errors.New("unknown or missing --mode. Allowed: auth, bootstrap, listen, send, verify, rooms, room-info, devices, logout")
	}

	return nil
}

func executeRoomsInfo(ctx context.Context, cli *client.Client, mode, rooms string, verbose bool) error {
	if mode == consts.ModeRooms {
		if !client.JSONMode {
			if _, err := fmt.Fprintf(stdout, "\nJoined Rooms:\n"); err != nil {
				return fmt.Errorf("failed to write to stdout: %w", err)
			}
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

func handleUpdate(ctx context.Context) error {
	httpClient := &http.Client{}
	if err := updater.Update(ctx, httpClient, AppVersion); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
