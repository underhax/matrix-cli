package client

import (
	"errors"
	"fmt"
	"strings"
)

// InteractiveDisabled globally disables interactive prompts in Machine Mode.
var InteractiveDisabled bool

// ReadPassword securely prompts the user for a password or recovery key.
// It hides the input characters and returns the trimmed string.
func ReadPassword(prompt string) (string, error) {
	if InteractiveDisabled {
		return "", errors.New("interactive input disabled in json mode")
	}

	if _, err := fmt.Fprint(stderr, prompt); err != nil {
		return "", fmt.Errorf("failed to print prompt: %w", err)
	}

	fd := getStdinFd()
	if !termIsTerminal(fd) {
		return "", errors.New("stdin is not a terminal, cannot securely read password")
	}

	bytePassword, err := termReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	if _, err := fmt.Fprintln(stderr); err != nil {
		return "", fmt.Errorf("failed to print newline: %w", err)
	}

	return strings.TrimSpace(string(bytePassword)), nil
}

var readPassword = ReadPassword
