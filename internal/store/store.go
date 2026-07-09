// Package store provides SQLite database initialization and lifecycle management
// for the matrix-cli's cryptographic and state persistence layer.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // Register the sqlite3 database driver.
)

// OpenDB initializes a SQLite connection tailored for high-concurrency environments.
// It explicitly enforces WAL journal mode and a busy timeout to prevent locking contention
// between independent CLI invocations (e.g., background listener and ad-hoc sender).
func OpenDB(ctx context.Context, path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_fk=true", path)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		closeErr := db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", errors.Join(err, closeErr))
	}

	return db, nil
}
