package store

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

func defaultRandRead(b []byte) (n int, err error) {
	n, _ = rand.Read(b)
	return n, nil
}

var randRead = defaultRandRead

// GetOrGeneratePickleKey reads the local symmetric encryption key from the specified path.
// If the file does not exist, it generates a cryptographically secure 32-byte key
// and saves it to the path with restricted file permissions.
func GetOrGeneratePickleKey(path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	if data, err := os.ReadFile(cleanPath); err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("pickle key file has invalid length: expected 32 bytes, got %d", len(data))
		}
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read pickle key: %w", err)
	}

	key := make([]byte, 32)
	if _, err := randRead(key); err != nil {
		return nil, fmt.Errorf("failed to generate random pickle key: %w", err)
	}

	if err := os.WriteFile(cleanPath, key, 0o600); err != nil {
		return nil, fmt.Errorf("failed to save pickle key: %w", err)
	}

	return key, nil
}
