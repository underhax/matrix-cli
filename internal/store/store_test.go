package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDB(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := OpenDB(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("db close error: %v", closeErr)
		}
	}()

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("expected ping to succeed, got %v", err)
	}
}

func TestOpenDB_InvalidPath(t *testing.T) {
	invalidPath := "/invalid_dir/test.db"

	_, err := OpenDB(context.Background(), invalidPath)
	if err == nil {
		t.Error("expected error for invalid DB path, got nil")
	}
}

func TestOpenDB_CanceledContext(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test2.db")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := OpenDB(ctx, dbPath)
	if err == nil {
		t.Error("expected error due to canceled context, got nil")
	}
}

func TestOpenDB_SqlOpenError(t *testing.T) {
	oldSQLOpen := sqlOpen
	defer func() { sqlOpen = oldSQLOpen }()
	sqlOpen = func(_, _ string) (*sql.DB, error) {
		return nil, context.DeadlineExceeded
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test3.db")

	_, err := OpenDB(context.Background(), dbPath)
	if err == nil {
		t.Error("expected error from sqlOpen failure, got nil")
	}
}

func TestDefaultSQLOpen_Error(t *testing.T) {
	_, err := defaultSQLOpen("invalid_driver_that_does_not_exist", "dummy_dsn")
	if err == nil {
		t.Error("expected error when using invalid driver, got nil")
	}
}
