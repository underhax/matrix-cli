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
		DeviceName:    "TestBot",
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

func TestLoad_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	sessionFile := filepath.Join(tempDir, "invalid.json")

	if err := os.WriteFile(sessionFile, []byte("{ bad json"), 0o600); err != nil {
		t.Fatalf("failed to write bad json: %v", err)
	}

	_, err := Load(sessionFile)
	if err == nil {
		t.Error("expected error loading invalid JSON, got nil")
	}
}

func TestSave_MarshalError(t *testing.T) {
	oldMarshal := jsonMarshalIndent
	defer func() { jsonMarshalIndent = oldMarshal }()
	jsonMarshalIndent = func(_ any, _, _ string) ([]byte, error) {
		return nil, os.ErrPermission
	}

	tempDir := t.TempDir()
	sessionFile := filepath.Join(tempDir, "session.json")
	session := &Session{}

	err := Save(sessionFile, session)
	if err == nil {
		t.Error("expected error from json marshal failure, got nil")
	}
}
