package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetOrGeneratePickleKey(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "pickle.key")

	key1, err := GetOrGeneratePickleKey(keyPath)
	if err != nil {
		t.Fatalf("failed to generate new key: %v", err)
	}
	if len(key1) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(key1))
	}

	data, readErr := os.ReadFile(filepath.Clean(keyPath))
	if readErr != nil {
		t.Fatalf("failed to read generated key file: %v", readErr)
	}
	if !bytes.Equal(data, key1) {
		t.Errorf("file content does not match generated key")
	}

	key2, err2 := GetOrGeneratePickleKey(keyPath)
	if err2 != nil {
		t.Fatalf("failed to read existing key: %v", err2)
	}
	if !bytes.Equal(key1, key2) {
		t.Errorf("expected existing key to match generated key")
	}

	invalidPath := filepath.Join(tempDir, "invalid.key")
	if writeErr := os.WriteFile(invalidPath, []byte("short"), 0o600); writeErr != nil {
		t.Fatalf("failed to write invalid key: %v", writeErr)
	}
	_, err = GetOrGeneratePickleKey(invalidPath)
	if err == nil {
		t.Errorf("expected error for invalid key length, got nil")
	} else if !strings.Contains(err.Error(), "invalid length") {
		t.Errorf("expected invalid length error, got: %v", err)
	}

	dirPath := filepath.Join(tempDir, "dir.key")
	if mkErr := os.Mkdir(dirPath, 0o700); mkErr != nil {
		t.Fatalf("failed to create directory: %v", mkErr)
	}
	_, err = GetOrGeneratePickleKey(dirPath)
	if err == nil {
		t.Errorf("expected error when path is a directory, got nil")
	}

	readOnlyDir := filepath.Join(tempDir, "readonly")
	if mkErr := os.Mkdir(readOnlyDir, 0o500); mkErr != nil {
		t.Fatalf("failed to create readonly directory: %v", mkErr)
	}
	unwriteablePath := filepath.Join(readOnlyDir, "unwriteable.key")
	_, err = GetOrGeneratePickleKey(unwriteablePath)
	if err == nil {
		t.Errorf("expected error when writing to readonly dir, got nil")
	} else if !strings.Contains(err.Error(), "failed to save pickle key") {
		t.Errorf("expected save error, got: %v", err)
	}
}

func TestGetOrGeneratePickleKey_RandError(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "pickle.key")

	oldRandRead := randRead
	defer func() { randRead = oldRandRead }()
	randRead = func(_ []byte) (n int, err error) {
		return 0, os.ErrPermission
	}

	_, err := GetOrGeneratePickleKey(keyPath)
	if err == nil {
		t.Errorf("expected error from randRead failure, got nil")
	} else if !strings.Contains(err.Error(), "failed to generate random pickle key") {
		t.Errorf("expected random key error, got: %v", err)
	}
}
