package validator

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"localhost_domain", localhost, false},
		{"valid_domain", "example.com", false},
		{"idn_domain", "пример.испытание", false},
		{"trailing_dot", "example.com.", false},
		{"too_short", "a", true},
		{"too_long", strings.Repeat("a", 254), true},
		{"no_dots", "example", true},
		{"label_too_long", strings.Repeat("a", 64) + ".com", true},
		{"hyphen_start", "-example.com", true},
		{"invalid_chars", "ex@mple.com", true},
		{"numeric_tld", "example.123", true},
		{"idn_invalid_punycode", "xn--a.example", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDomain() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDomainLabel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_label", "example", false},
		{"empty_label", "", true},
		{"too_long_label", strings.Repeat("b", 64), true},
		{"hyphen_start", "-abc", true},
		{"hyphen_end", "abc-", true},
		{"invalid_char", "ab@c", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomainLabel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDomainLabel() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateServerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_ip", "192.168.1.1", false},
		{"valid_ipv6", "[2001:db8::1]", false},
		{"valid_ip_port", "192.168.1.1:8080", false},
		{"valid_ipv6_port", "[2001:db8::1]:8448", false},
		{"valid_domain", "matrix.example.com", false},
		{"valid_domain_port", "matrix.example.com:443", false},
		{"invalid_port_range", "example.com:99999", true},
		{"invalid_port_alpha", "example.com:abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServerName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateServerName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_http", "http://example.com", false},
		{"valid_https_port", "https://example.com:8448", false},
		{"no_scheme_domain", "example.org", false},
		{"no_scheme_ip", "127.0.0.1", false},
		{"invalid_url_format", "http://[::1]:missing", true},
		{"invalid_domain", "http://invalid_domain", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRoomID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_old_format", "!room:example.com", false},
		{"valid_new_format", "!room123", false},
		{"missing_prefix", "room:example.com", true},
		{"empty_body", "!", true},
		{"empty_localpart", "!:example.com", true},
		{"empty_domain", "!room:", true},
		{"invalid_local_chars", "!ro*om:example.com", true},
		{"invalid_domain_chars", "!room:ex@mple.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoomID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRoomID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUserID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid_user", "@user:example.com", false},
		{"too_long", "@" + strings.Repeat("a", 250) + ":example.com", true},
		{"missing_prefix", "user:example.com", true},
		{"missing_domain", "@user", true},
		{"empty_local", "@:example.com", true},
		{"empty_domain", "@user:", true},
		{"invalid_local", "@us*r:example.com", true},
		{"invalid_domain", "@user:ex@mple.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUserID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePermissions(t *testing.T) {
	t.Cleanup(func() { statFile = defaultStatFile })

	tests := []struct {
		mock    func(string) (os.FileInfo, error)
		wantErr error
		name    string
	}{
		{
			name: "secure_0600",
			mock: func(string) (os.FileInfo, error) {
				return fakeFileInfo{perm: 0o600}, nil
			},
		},
		{
			name: "insecure_0644",
			mock: func(string) (os.FileInfo, error) {
				return fakeFileInfo{perm: 0o644}, nil
			},
			wantErr: ErrInsecurePermission,
		},
		{
			name: "file_not_exist",
			mock: func(string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			},
		},
		{
			name: "stat_error",
			mock: func(string) (os.FileInfo, error) {
				return nil, os.ErrPermission
			},
			wantErr: os.ErrPermission,
		},
	}

	t.Run("windows_bypass", func(t *testing.T) {
		originalOS := goOS
		t.Cleanup(func() { goOS = originalOS })
		goOS = "windows"
		statFile = func(string) (os.FileInfo, error) {
			return fakeFileInfo{perm: 0o644}, nil
		}
		if err := ValidatePermissions("/fake/path"); err != nil {
			t.Errorf("expected no error on windows, got %v", err)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statFile = tt.mock
			err := ValidatePermissions("/fake/path")
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

type fakeFileInfo struct{ perm os.FileMode }

func (f fakeFileInfo) Name() string      { return "fake" }
func (f fakeFileInfo) Size() int64       { return 0 }
func (f fakeFileInfo) Mode() os.FileMode { return f.perm }
func (f fakeFileInfo) ModTime() time.Time {
	return time.Time{}
}
func (f fakeFileInfo) IsDir() bool { return false }
func (f fakeFileInfo) Sys() any    { return nil }

func TestDefaultStatFile(t *testing.T) {
	tempDir := t.TempDir()

	secure := filepath.Join(tempDir, "secure.txt")
	if err := os.WriteFile(secure, []byte("ok"), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	info, err := defaultStatFile(secure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}

	_, err = defaultStatFile(filepath.Join(tempDir, "missing.txt"))
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
