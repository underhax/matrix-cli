package client

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestReadPassword(t *testing.T) {
	type termTestCase struct {
		name           string
		prompt         string
		expected       string
		termPassErr    error
		termPassReturn []byte
		stdoutErrNum   int
		isTerm         bool
		expectErr      bool
	}

	tests := []termTestCase{
		{
			name:           "success",
			prompt:         "Enter pass 1: ",
			isTerm:         true,
			termPassReturn: []byte("mysecret\n"),
			expected:       "mysecret",
			expectErr:      false,
		},
		{
			name:      "not_terminal",
			prompt:    "Enter pass 2: ",
			isTerm:    false,
			expectErr: true,
		},
		{
			name:        "term_read_error",
			prompt:      "Enter pass 3: ",
			isTerm:      true,
			termPassErr: errors.New("mock read error"),
			expectErr:   true,
		},
		{
			name:         "stdout_print_error",
			prompt:       "Enter pass 4: ",
			isTerm:       true,
			stdoutErrNum: 1,
			expectErr:    true,
		},
		{
			name:           "stdout_println_error",
			prompt:         "Enter pass 5: ",
			isTerm:         true,
			termPassReturn: []byte("pass"),
			stdoutErrNum:   2,
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origStdout := stdout
			origTermIsTerminal := termIsTerminal
			origTermReadPassword := termReadPassword
			origGetStdinFd := getStdinFd
			defer func() {
				stdout = origStdout
				termIsTerminal = origTermIsTerminal
				termReadPassword = origTermReadPassword
				getStdinFd = origGetStdinFd
			}()

			var outBuf bytes.Buffer
			if tt.stdoutErrNum > 0 {
				stdout = &errorWriter{failOnWriteNum: tt.stdoutErrNum}
			} else {
				stdout = &outBuf
			}

			getStdinFd = func() int { return 0 }
			termIsTerminal = func(_ int) bool { return tt.isTerm }
			termReadPassword = func(_ int) ([]byte, error) { return tt.termPassReturn, tt.termPassErr }

			res, err := ReadPassword(tt.prompt)

			if (err != nil) != tt.expectErr {
				t.Errorf("expected err=%v, got %v", tt.expectErr, err)
			}
			if err == nil && res != tt.expected {
				t.Errorf("expected res=%q, got %q", tt.expected, res)
			}
			if err == nil && !strings.Contains(outBuf.String(), tt.prompt) {
				t.Errorf("expected prompt %q in stdout, got %q", tt.prompt, outBuf.String())
			}
		})
	}
}
