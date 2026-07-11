package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleAuth_MissingCredentials(t *testing.T) {
	err := handleAuth(context.Background(), "http://localhost", "", "", "", "", "dummy.json")
	if err == nil {
		t.Error("Expected error due to missing user/pass, got nil")
	}
}

func TestHandleAuth_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_matrix/client/r0/login" || r.URL.Path == "/_matrix/client/v3/login" {
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"user_id":      "@user:example.com",
				"access_token": "test_token_123",
				"device_id":    "TEST_DEVICE_ID",
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				panic(err)
			}
			return
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	sessionFile := filepath.Join(tempDir, "session.json")

	err := handleAuth(context.Background(), server.URL, "user", "pass", "TestDevice", "", sessionFile)
	if err != nil {
		t.Fatalf("expected handleAuth to succeed, got %v", err)
	}

	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Error("expected session file to be created, but it does not exist")
	}
}
