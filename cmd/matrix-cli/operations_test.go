package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underhax/matrix-cli/internal/client"
	"github.com/underhax/matrix-cli/internal/config"
	"github.com/underhax/matrix-cli/internal/logger"

	"maunium.net/go/mautrix"
)

func TestHandleOperations_InvalidSession(t *testing.T) {
	ctx := context.Background()
	nopLog := logger.Nop()
	err := handleOperations(ctx, &nopLog, modeListen, "", "", "", false, "", false, "nonexistent.json", ":memory:", "pickle.key")
	if err == nil || !strings.Contains(err.Error(), "failed to load session") {
		t.Fatalf("expected failed to load session error, got %v", err)
	}
}

func TestHandleOperations_InvalidDB(t *testing.T) {
	tmp := t.TempDir()
	sessFile := filepath.Join(tmp, "session.json")
	sess := &config.Session{HomeserverURL: "http://127.0.0.1:4", UserID: "@user_op1:example.com", AccessToken: "tok1", DeviceID: "dev1"}
	err := config.Save(sessFile, sess)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	ctx := context.Background()
	nopLog := logger.Nop()
	err = handleOperations(ctx, &nopLog, modeListen, "", "", "", false, "", false, sessFile, "/invalid/db/path", "pickle.key")
	if err == nil || !strings.Contains(err.Error(), "database error") {
		t.Fatalf("expected database error, got %v", err)
	}
}

func TestHandleOperations_InvalidClient(t *testing.T) {
	tmp := t.TempDir()
	sessFile := filepath.Join(tmp, "session.json")
	sess := &config.Session{HomeserverURL: ":\x7f\x7f\x7f", UserID: "@user_op2:example.com", AccessToken: "tok2", DeviceID: "dev2"}
	err := config.Save(sessFile, sess)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	ctx := context.Background()
	nopLog := logger.Nop()
	err = handleOperations(ctx, &nopLog, modeListen, "", "", "", false, "", false, sessFile, ":memory:", filepath.Join(tmp, "p.key"))
	if err == nil || !strings.Contains(err.Error(), "client initialization failed") {
		t.Fatalf("expected client initialization error, got %v", err)
	}
}

func TestHandleOperations_DBCloseError(t *testing.T) {
	oldDBClose := dbClose
	defer func() { dbClose = oldDBClose }()
	dbClose = func(_ *sql.DB) error {
		return errors.New("mock db close error inside handleOperations")
	}

	tmp := t.TempDir()
	sessFile := filepath.Join(tmp, "session.json")
	sess := &config.Session{HomeserverURL: ":\x7f\x7f\x7f", UserID: "@user_db_err:example.com", AccessToken: "tok3", DeviceID: "dev3"}
	err := config.Save(sessFile, sess)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	ctx := context.Background()
	nopLog := logger.Nop()
	err = handleOperations(ctx, &nopLog, modeListen, "", "", "", false, "", false, sessFile, ":memory:", filepath.Join(tmp, "p.key"))
	if err == nil || !strings.Contains(err.Error(), "client initialization failed") {
		t.Fatalf("expected client initialization error, got %v", err)
	}
}

func TestHandleLogout(t *testing.T) {
	tmp := t.TempDir()
	sessFile := filepath.Join(tmp, "session.json")
	dbFile := filepath.Join(tmp, "db.sqlite3")
	pickleFile := filepath.Join(tmp, "pickle.key")

	sess := &config.Session{HomeserverURL: "http://127.0.0.1:5", UserID: "@user_logout:example.com", AccessToken: "tok_logout", DeviceID: "dev_logout"}
	err := config.Save(sessFile, sess)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}
	err = os.WriteFile(pickleFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("failed to write pickle key: %v", err)
	}

	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	nopLog := logger.Nop()
	err = handleOperations(ctx, &nopLog, modeLogout, "", "", "", false, "", false, sessFile, dbFile, pickleFile)
	if err != nil {
		t.Fatalf("expected logout to succeed, got %v", err)
	}

	if _, err := os.Stat(sessFile); !os.IsNotExist(err) {
		t.Errorf("expected session file to be deleted")
	}
	if _, err := os.Stat(pickleFile); !os.IsNotExist(err) {
		t.Errorf("expected pickle file to be deleted")
	}
}

func TestHandleLogout_Errors(t *testing.T) {
	oldDBClose := dbClose
	oldOSRemove := osRemove
	defer func() {
		dbClose = oldDBClose
		osRemove = oldOSRemove
	}()

	dbClose = func(_ *sql.DB) error {
		return errors.New("mock db close error")
	}
	osRemove = func(_ string) error {
		return errors.New("mock remove error")
	}

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNullRead, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull for read: %v", err)
	}
	stdout = devNullRead

	tmp := t.TempDir()
	sessFile := filepath.Join(tmp, "session.json")
	dbFile := filepath.Join(tmp, "db.sqlite3")
	pickleFile := filepath.Join(tmp, "pickle.key")

	sess := &config.Session{HomeserverURL: "http://127.0.0.1:6", UserID: "@user_logout_err:example.com", AccessToken: "test_token_invalid_1", DeviceID: "dev_logout_err"}
	err = config.Save(sessFile, sess)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}
	err = os.WriteFile(pickleFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("failed to write pickle key: %v", err)
	}

	ctx := context.Background()
	nopLog := logger.Nop()

	err = handleOperations(ctx, &nopLog, modeLogout, "", "", "", false, "", false, sessFile, dbFile, pickleFile)
	if err != nil {
		t.Fatalf("expected logout to succeed despite mock errors, got %v", err)
	}
}

func TestHandleOperations_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{}`))
		if err != nil {
			t.Errorf("mock response write error: %v", err)
		}
	}))
	defer mockServer.Close()

	tmp := t.TempDir()
	sessFile := filepath.Join(tmp, "session.json")
	dbFile := filepath.Join(tmp, "db.sqlite3")
	pickleFile := filepath.Join(tmp, "pickle.key")

	sess := &config.Session{HomeserverURL: mockServer.URL, UserID: "@user_succ:example.com", AccessToken: "tok_succ", DeviceID: "dev_succ"}
	err := config.Save(sessFile, sess)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	nopLog := logger.Nop()
	err = handleOperations(ctx, &nopLog, modeSend, "", "", "", false, "", false, sessFile, dbFile, pickleFile)
	if err == nil || err.Error() != "--rooms and --message are required for send mode" {
		t.Errorf("expected send validation error from executeMode, got %v", err)
	}
}

func TestExecuteMode_Validation(t *testing.T) {
	macli, err := mautrix.NewClient("http://127.0.0.1:1", "@u1:example.com", "tok")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	cli := &client.Client{Matrix: macli}

	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	err = executeMode(ctx, cli, modeSend, "", "", "", false, "", false)
	if err == nil || err.Error() != "--rooms and --message are required for send mode" {
		t.Errorf("expected send error, got %v", err)
	}

	err = executeRoomsInfo(ctx, cli, modeRoomInfo, "", false)
	if err == nil || err.Error() != "--rooms is required for room-info mode" {
		t.Errorf("expected room-info error, got %v", err)
	}
}

func TestExecuteMode_Execution(t *testing.T) {
	macli, err := mautrix.NewClient("http://127.0.0.1:3", "@u3:example.com", "tok")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	cli := &client.Client{Matrix: macli}
	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	err = executeMode(ctx, cli, modeVerify, "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "verify mode error") {
		t.Errorf("expected verify error, got %v", err)
	}
	err = executeMode(ctx, cli, modeBootstrap, "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "bootstrap error") {
		t.Errorf("expected bootstrap error, got %v", err)
	}
	err = executeMode(ctx, cli, modeListen, "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "listener error") {
		t.Errorf("expected listener error, got %v", err)
	}
	err = executeRoomsInfo(ctx, cli, modeRooms, "", false)
	if err == nil || !strings.Contains(err.Error(), "rooms list error") {
		t.Errorf("expected rooms list error, got %v", err)
	}
}

func TestExecuteMode_Execution_Part2(t *testing.T) {
	macli, err := mautrix.NewClient("http://127.0.0.1:2", "@u2:example.com", "tok2")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	cli := &client.Client{Matrix: macli}
	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	err = executeRoomsInfo(ctx, cli, modeRoomInfo, "!room:example.com", false)
	if err != nil {
		t.Errorf("expected room info to return nil even on network errors, got %v", err)
	}

	err = executeMode(ctx, cli, modeDevices, "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "devices fetch error") {
		t.Errorf("expected devices fetch error, got %v", err)
	}

	err = executeMode(ctx, cli, "invalid-mode", "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "unknown or missing") {
		t.Errorf("expected unknown mode error, got %v", err)
	}

	err = executeMode(ctx, cli, modeRoomInfo, " ", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "room info error") {
		t.Errorf("expected room info error from empty string, got %v", err)
	}

	err = executeMode(ctx, cli, modeSend, " ", "hello", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "send error") {
		t.Errorf("expected send error, got %v", err)
	}
}

func TestExecuteMode_Execution_Part3(t *testing.T) {
	macli, err := mautrix.NewClient("http://127.0.0.1:7", "@u7:example.com", "tok7")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	cli := &client.Client{Matrix: macli}
	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	err = executeMode(ctx, cli, modeRooms, "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "rooms list error") {
		t.Errorf("expected rooms list error from executeMode, got %v", err)
	}

	err = executeMode(ctx, cli, modeRoomInfo, "!room:example.com", "", "", false, "", false)
	if err != nil {
		t.Errorf("expected room info via executeMode to return nil, got %v", err)
	}

	if closeErr := devNull.Close(); closeErr != nil {
		t.Fatalf("failed to close devnull: %v", closeErr)
	}
	err = executeMode(ctx, cli, modeRooms, "", "", "", false, "", false)
	if err == nil || !strings.Contains(err.Error(), "failed to write to stdout") {
		t.Errorf("expected stdout write error, got %v", err)
	}
}

func TestExecuteMode_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{}`))
		if err != nil {
			t.Errorf("mock write error: %v", err)
		}
	}))
	defer mockServer.Close()

	macli, err := mautrix.NewClient(mockServer.URL, "@u_succ_ex:example.com", "tok_ex")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	cli := &client.Client{Matrix: macli}
	ctx := context.Background()

	oldStdout := stdout
	defer func() { stdout = oldStdout }()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("failed to open devnull: %v", err)
	}
	stdout = devNull

	err = executeMode(ctx, cli, modeDevices, "", "", "", false, "", false)
	if err != nil {
		t.Errorf("expected devices fetch to succeed, got %v", err)
	}

	err = executeMode(ctx, cli, modeRooms, "", "", "", false, "", false)
	if err != nil {
		t.Errorf("expected rooms list to succeed, got %v", err)
	}
}
