package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin_Success(t *testing.T) {
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
		t.Fatalf("unexpected request to %s", r.URL.Path)
	}))
	defer server.Close()

	ctx := context.Background()
	session, err := Login(ctx, server.URL, "user", "pass", "TestBot")
	if err != nil {
		t.Fatalf("expected login to succeed, got %v", err)
	}

	if session.UserID != "@user:example.com" {
		t.Errorf("expected user ID @user:example.com, got %s", session.UserID)
	}
	if session.AccessToken != "test_token_123" {
		t.Errorf("expected token test_token_123, got %s", session.AccessToken)
	}
	if session.DeviceID != "TEST_DEVICE_ID" {
		t.Errorf("expected device ID TEST_DEVICE_ID, got %s", session.DeviceID)
	}
	if session.DeviceName != "TestBot" {
		t.Errorf("expected device Name TestBot, got %s", session.DeviceName)
	}
}

func TestLogin_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_matrix/client/r0/login" || r.URL.Path == "/_matrix/client/v3/login" {
			w.WriteHeader(http.StatusForbidden)
			resp := map[string]any{
				"errcode": "M_FORBIDDEN",
				"error":   "Invalid password",
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				panic(err)
			}
			return
		}
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := Login(ctx, server.URL, "user", "wrong_pass", "TestBot")
	if err == nil {
		t.Error("expected login to fail, got nil")
	}
}
