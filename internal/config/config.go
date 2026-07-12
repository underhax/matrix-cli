// Package config manages the persistent session state for the matrix-cli client,
// handling secure serialization and deserialization of authentication credentials to disk.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/underhax/matrix-cli/internal/consts"
)

// Session represents the persisted matrix authentication state required for client initialization
// without performing a new login request to the homeserver.
type Session struct {
	HomeserverURL string `json:"homeserver_url"`
	UserID        string `json:"user_id"`
	AccessToken   string `json:"access_token"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
}

// Load reads the session configuration from a sanitized file path.
func Load(path string) (*Session, error) {
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &session, nil
}

var jsonMarshalIndent = json.MarshalIndent

// Save writes the current authentication state to a sanitized file path for future executions.
func Save(path string, session *Session) error {
	disk := map[string]string{
		"homeserver_url":      session.HomeserverURL,
		consts.KeyUserID:      session.UserID,
		consts.KeyAccessToken: session.AccessToken,
		consts.KeyDeviceID:    session.DeviceID,
		consts.KeyDeviceName:  session.DeviceName,
	}

	data, err := jsonMarshalIndent(disk, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	cleanPath := filepath.Clean(path)

	if err := os.WriteFile(cleanPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}
