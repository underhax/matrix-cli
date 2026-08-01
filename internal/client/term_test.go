package client

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestReadPassword(t *testing.T) {
	type termTestCase struct {
		termPassErr         error
		mockErr             error
		name                string
		prompt              string
		expected            string
		termPassReturn      []byte
		stderrErrNum        int
		isTerm              bool
		interactiveDisabled bool
		expectErr           bool
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
			name:         "stderr_print_error",
			prompt:       "Enter pass 4: ",
			isTerm:       true,
			stderrErrNum: 1,
			mockErr:      errors.New("failed to print prompt to stderr"),
			expectErr:    true,
		},
		{
			name:           "stderr_println_error",
			prompt:         "Enter pass 5: ",
			isTerm:         true,
			termPassReturn: []byte("pass"),
			stderrErrNum:   2,
			mockErr:        errors.New("failed to print newline to stderr"),
			expectErr:      true,
		},
		{
			name:                "interactive_disabled",
			prompt:              "Enter pass 6: ",
			interactiveDisabled: true,
			expectErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origStderr := stderr
			origTermIsTerminal := termIsTerminal
			origTermReadPassword := termReadPassword
			origGetStdinFd := getStdinFd
			origInteractiveDisabled := InteractiveDisabled
			defer func() {
				stderr = origStderr
				termIsTerminal = origTermIsTerminal
				termReadPassword = origTermReadPassword
				getStdinFd = origGetStdinFd
				InteractiveDisabled = origInteractiveDisabled
			}()

			InteractiveDisabled = tt.interactiveDisabled

			var errBuf bytes.Buffer
			if tt.stderrErrNum > 0 {
				stderr = &errorWriter{failOnWriteNum: tt.stderrErrNum, mockErr: tt.mockErr}
			} else {
				stderr = &errBuf
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
			if err == nil && !strings.Contains(errBuf.String(), tt.prompt) {
				t.Errorf("expected prompt %q in stderr, got %q", tt.prompt, errBuf.String())
			}
		})
	}
}
