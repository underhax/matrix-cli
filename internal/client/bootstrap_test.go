package client

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeBase64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		encoder *base64.Encoding
		want    string
	}{
		{"StdEncoding", base64.StdEncoding.EncodeToString([]byte("test1")), base64.StdEncoding, "test1"},
		{"RawStdEncoding", base64.RawStdEncoding.EncodeToString([]byte("test2")), base64.RawStdEncoding, "test2"},
		{"URLEncoding", base64.URLEncoding.EncodeToString([]byte("test3?")), base64.URLEncoding, "test3?"},
		{"RawURLEncoding", base64.RawURLEncoding.EncodeToString([]byte("test4?")), base64.RawURLEncoding, "test4?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeBase64(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", string(got), tt.want)
			}
		})
	}
}

func TestBootstrap_MissingParams(t *testing.T) {
	c := &Client{}
	err := c.Bootstrap(context.Background(), false, "")
	expectedErrPrefix := "failed to prompt for recovery key"
	if err == nil || !strings.HasPrefix(err.Error(), expectedErrPrefix) {
		t.Errorf("unexpected error message: %v", err)
	}
}
