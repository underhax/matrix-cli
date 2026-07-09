package store

import (
	"context"
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
