package client

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
)

func TestWrapErr(t *testing.T) {
	if err := wrapErr(nil, "test"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	baseErr := errors.New("base error")
	wrapped := wrapErr(baseErr, "wrapped: %w")
	if wrapped == nil || wrapped.Error() != "wrapped: base error" {
		t.Errorf("unexpected wrap result: %v", wrapped)
	}
}

func TestDefaultGetOlmMachine(t *testing.T) {
	c := &Client{}
	mach := defaultGetOlmMachine(c)
	if mach != nil {
		t.Errorf("expected nil machine, got %v", mach)
	}

	cWithCrypto := &Client{
		Crypto: &cryptohelper.CryptoHelper{},
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic, but did not panic")
			}
		}()
		defaultGetOlmMachine(cWithCrypto)
	}()
}

type mockRows struct {
	dbutil.Rows
	closeErr error
}

func (m *mockRows) Close() error {
	return m.closeErr
}

func TestDefaultRowsClose(t *testing.T) {
	if err := defaultRowsClose(nil); err != nil {
		t.Errorf("expected nil error for nil rows, got %v", err)
	}

	successRows := &mockRows{closeErr: nil}
	if err := defaultRowsClose(successRows); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	failRows := &mockRows{closeErr: errors.New("close fail")}
	if err := defaultRowsClose(failRows); err == nil || err.Error() != "failed to close rows: close fail" {
		t.Errorf("unexpected error result: %v", err)
	}
}

type errWriter struct{}

func (errWriter) Write(_ []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func TestStderrWrappers(t *testing.T) {
	oldStderr := stderr
	defer func() { stderr = oldStderr }()

	var buf bytes.Buffer
	stderr = &buf

	fprintfStderr("test %d", 1)
	if buf.String() != "test 1" {
		t.Errorf("fprintfStderr output mismatch: got %q", buf.String())
	}

	buf.Reset()
	fprintlnStderr("test")
	if buf.String() != "test\n" {
		t.Errorf("fprintlnStderr output mismatch: got %q", buf.String())
	}

	stderr = errWriter{}
	fprintfStderr("test")
	fprintlnStderr("test")
}

func TestDefaultClearCryptoCache_NilAndMemory(t *testing.T) {
	err := defaultClearCryptoCache(context.Background(), nil, "user")
	if err == nil || err.Error() != "machine is nil" {
		t.Errorf("expected machine is nil, got %v", err)
	}

	machMemory := &crypto.OlmMachine{CryptoStore: &crypto.MemoryStore{}}
	if errMem := defaultClearCryptoCache(context.Background(), machMemory, "user"); errMem != nil {
		t.Errorf("expected nil for memory store, got %v", errMem)
	}
}

func TestDefaultClearCryptoCache_SQL(t *testing.T) {
	sqlStore := &crypto.SQLCryptoStore{}
	sqldb, errSQL := sql.Open("sqlite3", ":memory:")
	if errSQL != nil {
		t.Fatalf("sql.Open failed: %v", errSQL)
	}
	defer func() {
		if errClose := sqldb.Close(); errClose != nil {
			t.Logf("close error: %v", errClose)
		}
	}()
	db, errDb := dbutil.NewWithDB(sqldb, "sqlite3")
	if errDb != nil {
		t.Fatalf("NewWithDB failed: %v", errDb)
	}
	sqlStore.DB = db
	machSQL := &crypto.OlmMachine{CryptoStore: sqlStore}

	t.Run("fails_on_keys_table", func(t *testing.T) {
		err := defaultClearCryptoCache(context.Background(), machSQL, "user")
		if err == nil || !strings.Contains(err.Error(), "delete keys:") {
			t.Errorf("expected delete keys error, got %v", err)
		}
	})

	if _, errExec := db.Exec(context.Background(), "CREATE TABLE crypto_cross_signing_keys (user_id TEXT)"); errExec != nil {
		t.Fatalf("exec failed: %v", errExec)
	}

	t.Run("fails_on_devices_table", func(t *testing.T) {
		err := defaultClearCryptoCache(context.Background(), machSQL, "user")
		if err == nil || !strings.Contains(err.Error(), "delete devices:") {
			t.Errorf("expected delete devices error, got %v", err)
		}
	})

	if _, errExec := db.Exec(context.Background(), "CREATE TABLE crypto_device (user_id TEXT)"); errExec != nil {
		t.Fatalf("exec failed: %v", errExec)
	}

	t.Run("fails_on_signatures_table", func(t *testing.T) {
		err := defaultClearCryptoCache(context.Background(), machSQL, "user")
		if err == nil || !strings.Contains(err.Error(), "delete signatures:") {
			t.Errorf("expected delete signatures error, got %v", err)
		}
	})

	if _, errExec := db.Exec(context.Background(), "CREATE TABLE crypto_cross_signing_signatures (signed_user_id TEXT, signer_user_id TEXT)"); errExec != nil {
		t.Fatalf("exec failed: %v", errExec)
	}

	t.Run("success", func(t *testing.T) {
		err := defaultClearCryptoCache(context.Background(), machSQL, "user")
		if err != nil {
			t.Errorf("expected success, got %v", err)
		}
	})
}
