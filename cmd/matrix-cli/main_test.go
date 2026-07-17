package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underhax/matrix-cli/internal/consts"
)

func TestHandleAuth_MissingCredentials(t *testing.T) {
	err := handleAuth(context.Background(), "http://localhost", "", "", "", "", "dummy.json")
	if err == nil {
		t.Error("Expected error due to missing user/pass, got nil")
	}
}

func TestHandleAuth_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_matrix/client/r0/login" || r.URL.Path == "/_matrix/client/v3/login" {
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				consts.KeyUserID:      "@alice:example.com",
				consts.KeyAccessToken: "token_alice_123",
				consts.KeyDeviceID:    "DEV_ALICE_ID",
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				panic(err)
			}
			return
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	sessionFile := filepath.Join(tempDir, "session.json")

	err := handleAuth(context.Background(), server.URL, "alice", "pass_alice", "DeviceAlice", "", sessionFile)
	if err != nil {
		t.Fatalf("expected handleAuth to succeed, got %v", err)
	}

	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Error("expected session file to be created, but it does not exist")
	}
}

type errorWriter struct{}

func (e errorWriter) Write(_ []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func TestHandleAuth_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := map[string]any{
			consts.KeyUserID:      "@bob:example.net",
			consts.KeyAccessToken: "token_bob_456",
			consts.KeyDeviceID:    "DEV_BOB_ID",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
	}))
	defer server.Close()

	t.Run("config_save_fails", func(t *testing.T) {
		err := handleAuth(context.Background(), server.URL, "bob", "pass_bob", "DeviceBob", "", t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "failed to save session") {
			t.Errorf("expected failed to save session error, got %v", err)
		}
	})

	t.Run("filepath_abs_fails", func(t *testing.T) {
		oldAbs := filepathAbs
		filepathAbs = func(_ string) (string, error) {
			return "", errors.New("abs error")
		}
		defer func() { filepathAbs = oldAbs }()

		tempDir := t.TempDir()
		sessionFile := filepath.Join(tempDir, "session.json")
		err := handleAuth(context.Background(), server.URL, "charlie", "pass_charlie", "DeviceCharlie", "", sessionFile)
		if err != nil {
			t.Errorf("did not expect error, got %v", err)
		}
	})

	t.Run("stdout_write_fails", func(t *testing.T) {
		oldStdout := stdout
		stdout = errorWriter{}
		defer func() { stdout = oldStdout }()

		tempDir := t.TempDir()
		sessionFile := filepath.Join(tempDir, "session.json")
		err := handleAuth(context.Background(), server.URL, "dave", "pass_dave", "DeviceDave", "", sessionFile)
		if err != nil {
			t.Errorf("did not expect error, got %v", err)
		}
	})
}

func TestGetDefaultDataDir(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	t.Setenv("USERPROFILE", "")

	dir := getDefaultDataDir()
	if dir != "." {
		t.Errorf("Expected '.' when env vars are unset, got %q", dir)
	}

	t.Setenv("XDG_CONFIG_HOME", "/fake/xdg")
	if dir := getDefaultDataDir(); dir != filepath.Join(string(filepath.Separator), "fake", "xdg", "matrix-cli") {
		t.Errorf("Expected XDG path, got %q", dir)
	}

	oldGOOS := runtimeGOOS
	defer func() { runtimeGOOS = oldGOOS }()

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/fake/home")

	runtimeGOOS = "darwin"
	if dir := getDefaultDataDir(); dir != filepath.Join(string(filepath.Separator), "fake", "home", "Library", "Application Support", "matrix-cli") {
		t.Errorf("Expected macOS config path, got %q", dir)
	}

	runtimeGOOS = "linux"
	if dir := getDefaultDataDir(); dir != filepath.Join(string(filepath.Separator), "fake", "home", ".config", "matrix-cli") {
		t.Errorf("Expected Linux config path, got %q", dir)
	}

	t.Setenv("HOME", "")
	t.Setenv("AppData", "/fake/appdata")
	if dir := getDefaultDataDir(); dir != filepath.Join(string(filepath.Separator), "fake", "appdata", "matrix-cli") {
		t.Errorf("Expected AppData path, got %q", dir)
	}
}

func TestMainFunc(t *testing.T) {
	exited := false
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	}
	defer func() { osExit = os.Exit }()

	os.Args = []string{"matrix-cli", flagMode, "invalid-mode-for-test"}
	main()

	if !exited {
		t.Error("expected main to call osExit on error")
	}
}

func TestRun(t *testing.T) {
	tmpDir := t.TempDir()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	tests := []struct {
		name      string
		errString string
		args      []string
		wantErr   bool
	}{
		{
			name:    "no args",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "help flag",
			args:    []string{"-h"},
			wantErr: false,
		},
		{
			name:      "unknown mode",
			errString: "unknown mode: invalid_mode",
			args:      []string{flagMode, "invalid_mode"},
			wantErr:   true,
		},
		{
			name:      "invalid flag",
			errString: "flag provided but not defined",
			args:      []string{"--invalid-flag"},
			wantErr:   true,
		},
		{
			name:      "invalid data dir (covers verbose and debug flags)",
			errString: "failed to create data directory",
			args:      []string{flagMode, modeAuth, flagVerbose, flagDebug, flagDataDir, "/dev/null/invalid"},
			wantErr:   true,
		},
		{
			name:      "debug level 2",
			errString: "failed to create data directory",
			args:      []string{flagMode, modeAuth, "--debug=2", flagDataDir, "/dev/null/invalid"},
			wantErr:   true,
		},
		{
			name:    "validation fails inside run",
			args:    []string{flagMode, modeAuth, flagServer, "http://[::1]:err", flagDataDir, tmpDir},
			wantErr: false,
		},
		{
			name:    "auth mode calls handleAuth",
			args:    []string{flagMode, modeAuth, flagServer, mockServer.URL, flagDataDir, tmpDir},
			wantErr: true,
		},
		{
			name:      "operations mode calls handleOperations",
			errString: "failed to load session",
			args:      []string{flagMode, modeListen, flagDataDir, tmpDir},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStderr := os.Stderr
			devNull, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatalf("failed to open devnull: %v", err)
			}
			os.Stderr = devNull
			defer func() {
				os.Stderr = oldStderr
				if closeErr := devNull.Close(); closeErr != nil {
					t.Errorf("failed to close devnull: %v", closeErr)
				}
			}()

			err = run(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errString != "" {
				if !strings.Contains(err.Error(), tt.errString) {
					t.Errorf("run() error = %v, expected to contain %q", err, tt.errString)
				}
			}
		})
	}
}
