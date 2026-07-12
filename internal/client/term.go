package client

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ReadPassword securely prompts the user for a password or recovery key.
// It hides the input characters and returns the trimmed string.
func ReadPassword(prompt string) (string, error) {
	if _, err := fmt.Fprint(os.Stdout, prompt); err != nil {
		return "", fmt.Errorf("failed to print prompt: %w", err)
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("stdin is not a terminal, cannot securely read password")
	}

	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	if _, err := fmt.Fprintln(os.Stdout); err != nil {
		return "", fmt.Errorf("failed to print newline: %w", err)
	}

	return strings.TrimSpace(string(bytePassword)), nil
}

var readPassword = ReadPassword
