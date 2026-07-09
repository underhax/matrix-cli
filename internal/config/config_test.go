package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	sessionFile := filepath.Join(tempDir, "session.json")

	session := &Session{
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@user:example.com",
		AccessToken:   "test_token",
		DeviceID:      "TEST_DEVICE",
	}

	if err := Save(sessionFile, session); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	loadedSession, err := Load(sessionFile)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	if loadedSession.HomeserverURL != session.HomeserverURL {
		t.Errorf("expected URL %q, got %q", session.HomeserverURL, loadedSession.HomeserverURL)
	}
	if loadedSession.UserID != session.UserID {
		t.Errorf("expected UserID %q, got %q", session.UserID, loadedSession.UserID)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	sessionFile := filepath.Join(tempDir, "non_existent.json")

	_, err := Load(sessionFile)
	if err == nil {
		t.Error("expected error loading non-existent file, got nil")
	}
}

func TestSave_InvalidPath(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "is_a_dir")
	if err := os.Mkdir(invalidPath, 0o750); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	session := &Session{
		HomeserverURL: "https://matrix.example.com",
	}

	err := Save(invalidPath, session)
	if err == nil {
		t.Error("expected error saving to a directory path, got nil")
	}
}
