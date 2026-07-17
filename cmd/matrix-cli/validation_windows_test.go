//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateInputInsecurePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	badPermFile := filepath.Join(tmpDir, "bad.txt")
	if err := os.WriteFile(badPermFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to create temporary test file: %v", err)
	}

	got := validateInput(modeListen, "", "", "", badPermFile, badPermFile, badPermFile)
	if len(got) != 0 {
		t.Errorf("validateInput() on windows should skip permission checks, got = %v", got)
	}
}
