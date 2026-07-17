// Package validator provides input validation functions for CLI arguments and environment variables.
package validator

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

var goOS = runtime.GOOS

const localhost = "localhost"

// Validation errors returned by the package.
var (
	ErrInvalidURL         = errors.New("invalid URL format")
	ErrInvalidPort        = errors.New("port must be between 1 and 65535")
	ErrInvalidDomain      = errors.New("invalid domain structure")
	ErrInvalidRoomID      = errors.New("invalid room ID format")
	ErrInvalidUserID      = errors.New("invalid user ID format")
	ErrInsecurePermission = errors.New("insecure file permissions, expected 0600")
)

// ValidateDomain ensures the string is a valid domain name.
func ValidateDomain(domain string) error {
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)

	if domain == localhost {
		return nil
	}

	domain = strings.TrimSuffix(domain, ".")

	asciiDomain, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		return ErrInvalidDomain
	}

	if len(asciiDomain) < 3 || len(asciiDomain) > 253 {
		return ErrInvalidDomain
	}

	parts := strings.Split(asciiDomain, ".")
	if len(parts) < 2 {
		return ErrInvalidDomain
	}

	for _, part := range parts {
		if err := validateDomainLabel(part); err != nil {
			return err
		}
	}

	if isNumericLabel(parts[len(parts)-1]) {
		return ErrInvalidDomain
	}

	return nil
}

func validateDomainLabel(part string) error {
	pl := len(part)
	if pl < 1 || pl > 63 {
		return ErrInvalidDomain
	}
	if part[0] == '-' || part[pl-1] == '-' {
		return ErrInvalidDomain
	}
	for i := range len(part) {
		c := part[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return ErrInvalidDomain
		}
	}
	return nil
}

func isNumericLabel(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ValidateServerName ensures the string is a valid IP or domain, optionally with a port.
func ValidateServerName(name string) error {
	host, portStr, err := net.SplitHostPort(name)
	if err != nil {
		host = name
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return ErrInvalidPort
		}
	}

	cleanHost := strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if net.ParseIP(cleanHost) != nil {
		return nil
	}

	return ValidateDomain(host)
}

// ValidateURL ensures the string is a valid HTTP/HTTPS URL or server name.
func ValidateURL(u string) error {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		parsed, err := url.ParseRequestURI(u)
		if err != nil {
			return ErrInvalidURL
		}
		return ValidateServerName(parsed.Host)
	}
	return ValidateServerName(u)
}

// ValidateRoomID ensures the room ID adheres to Matrix specifications.
func ValidateRoomID(roomID string) error {
	if !strings.HasPrefix(roomID, "!") {
		return ErrInvalidRoomID
	}
	body := roomID[1:]
	if body == "" {
		return ErrInvalidRoomID
	}
	parts := strings.SplitN(body, ":", 2)
	if parts[0] == "" || !isValidRoomLocalpart(parts[0]) {
		return ErrInvalidRoomID
	}
	if len(parts) == 2 {
		if parts[1] == "" {
			return ErrInvalidRoomID
		}
		if err := ValidateServerName(parts[1]); err != nil {
			return ErrInvalidRoomID
		}
	}
	return nil
}

func isValidRoomLocalpart(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '=' || c == '/' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// ValidateUserID ensures the user ID adheres to Matrix specifications.
func ValidateUserID(userID string) error {
	if len(userID) > 255 {
		return ErrInvalidUserID
	}
	if !strings.HasPrefix(userID, "@") {
		return ErrInvalidUserID
	}
	body := userID[1:]
	parts := strings.SplitN(body, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !isValidUserLocalpart(parts[0]) {
		return ErrInvalidUserID
	}
	if err := ValidateServerName(parts[1]); err != nil {
		return ErrInvalidUserID
	}
	return nil
}

func isValidUserLocalpart(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '=' || c == '/' || c == '+' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func defaultStatFile(name string) (os.FileInfo, error) {
	info, err := os.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	return info, nil
}

var statFile = defaultStatFile

// ValidatePermissions ensures the file at the given path has secure (0o600) permissions.
func ValidatePermissions(filePath string) error {
	stat, err := statFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if goOS == "windows" {
		return nil
	}

	if stat.Mode().Perm() != 0o600 {
		return ErrInsecurePermission
	}
	return nil
}
