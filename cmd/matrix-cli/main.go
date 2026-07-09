// Package main is the entry point for the matrix-cli application.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"matrix-cli/internal/client"
	"matrix-cli/internal/config"
	"matrix-cli/internal/store"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "Execution mode: auth, listen, send, verify")
	server := flag.String("server", "https://matrix.org", "Homeserver URL (for auth)")
	user := flag.String("user", "", "Matrix user ID (for auth)")
	pass := flag.String("pass", "", "Matrix password (for auth)")
	device := flag.String("device", "", "Device display name (for auth)")
	room := flag.String("room", "", "Target room ID (for send)")
	msg := flag.String("message", "", "Message body (for send)")
	dataDir := flag.String("data-dir", ".", "Directory to store session and database files")

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	sessionFile := filepath.Join(*dataDir, "session.json")
	dbFile := filepath.Join(*dataDir, "crypto.db")

	if *mode == "auth" {
		return handleAuth(ctx, *server, *user, *pass, *device, sessionFile)
	}

	return handleOperations(ctx, *mode, *room, *msg, sessionFile, dbFile)
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

	_, _ = fmt.Fprintln(os.Stderr, "Authentication successful. Session saved.")
	return nil
}

func handleOperations(ctx context.Context, mode, room, msg, sessionFile, dbFile string) error {
	session, err := config.Load(sessionFile)
	if err != nil {
		return fmt.Errorf("failed to load session (run --mode auth first): %w", err)
	}

	db, err := store.OpenDB(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to close database: %v\n", closeErr)
		}
	}()

	cli, err := client.New(ctx, session, db)
	if err != nil {
		return fmt.Errorf("client initialization failed: %w", err)
	}

	return executeMode(ctx, cli, mode, room, msg)
}

func executeMode(ctx context.Context, cli *client.Client, mode, room, msg string) error {
	switch mode {
	case "listen":
		if err := cli.Listen(ctx); err != nil {
			return fmt.Errorf("listener error: %w", err)
		}
	case "send":
		if room == "" || msg == "" {
			return errors.New("--room and --message are required for send mode")
		}
		if err := cli.Send(ctx, room, msg); err != nil {
			return fmt.Errorf("send error: %w", err)
		}
	case "verify":
		if err := cli.Verify(ctx); err != nil {
			return fmt.Errorf("verify mode error: %w", err)
		}
	default:
		return errors.New("unknown or missing --mode. Allowed: auth, listen, send, verify")
	}
	return nil
}
