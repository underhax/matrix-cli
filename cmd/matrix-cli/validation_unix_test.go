//go:build unix

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateInputInsecurePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	badPermFile := filepath.Join(tmpDir, "bad.txt")
	var insecurePerm os.FileMode = 0o644
	if err := os.WriteFile(badPermFile, []byte("data"), insecurePerm); err != nil {
		t.Fatalf("failed to create temporary test file: %v", err)
	}
	if err := os.Chmod(badPermFile, insecurePerm); err != nil {
		t.Fatalf("failed to set test file permissions: %v", err)
	}

	got := validateInput(modeListen, "", "", "", badPermFile, badPermFile, badPermFile)
	want := []string{
		`insecure permissions on "` + badPermFile + `" (expected 0600)`,
		`insecure permissions on "` + badPermFile + `" (expected 0600)`,
		`insecure permissions on "` + badPermFile + `" (expected 0600)`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("validateInput()\ngot  = %v\nwant = %v", got, want)
	}
}
