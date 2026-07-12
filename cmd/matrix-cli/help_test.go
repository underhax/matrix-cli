package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, f func()) string {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stderr = w

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r); err != nil {
			t.Errorf("io.Copy failed: %v", err)
		}
		outC <- buf.String()
	}()

	f()

	if err := w.Close(); err != nil {
		t.Fatalf("w.Close failed: %v", err)
	}
	os.Stderr = oldStderr
	return <-outC
}

func TestPrintUsage(t *testing.T) {
	tests := []struct {
		name     string
		modeVal  string
		wantText string
	}{
		{"ModeAuth", modeAuth, "Usage: matrix-cli --mode auth"},
		{"ModeBootstrap", modeBootstrap, "Usage: matrix-cli --mode bootstrap"},
		{"ModeListen", modeListen, "Usage: matrix-cli --mode listen"},
		{"ModeSend", modeSend, "Usage: matrix-cli --mode send"},
		{"ModeVerify", modeVerify, "Usage: matrix-cli --mode verify"},
		{"ModeRooms", modeRooms, "Usage: matrix-cli --mode rooms"},
		{"ModeRoomInfo", modeRoomInfo, "Usage: matrix-cli --mode room-info"},
		{"ModeDevices", modeDevices, "Usage: matrix-cli --mode devices"},
		{"ModeLogout", modeLogout, "Usage: matrix-cli --mode logout"},
		{"UnknownMode", "unknown-mode-123", "matrix-cli - A headless Matrix client"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(t, func() {
				printUsage(tt.modeVal)
			})

			if !strings.Contains(output, tt.wantText) {
				t.Errorf("printUsage(%q) output does not contain expected text %q", tt.modeVal, tt.wantText)
			}
		})
	}
}
