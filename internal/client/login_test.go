package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"matrix-cli/internal/consts"
)

func TestLogin_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_matrix/client/r0/login" || r.URL.Path == "/_matrix/client/v3/login" {
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				consts.KeyUserID:      "@bot:example.com",
				consts.KeyAccessToken: "token_bot_1",
				consts.KeyDeviceID:    "DEV_BOT_1",
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
	session, err := Login(ctx, server.URL, "bot_user", "bot_pass", "BotDevice", "")
	if err != nil {
		t.Fatalf("expected login to succeed, got %v", err)
	}

	if session.UserID != "@bot:example.com" {
		t.Errorf("expected user ID @bot:example.com, got %s", session.UserID)
	}
	if session.AccessToken != "token_bot_1" {
		t.Errorf("expected token token_bot_1, got %s", session.AccessToken)
	}
	if session.DeviceID != "DEV_BOT_1" {
		t.Errorf("expected device ID DEV_BOT_1, got %s", session.DeviceID)
	}
	if session.DeviceName != "BotDevice" {
		t.Errorf("expected device Name BotDevice, got %s", session.DeviceName)
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
	_, err := Login(ctx, server.URL, "wrong_user", "wrong_pass", "FailDevice", "")
	if err == nil {
		t.Error("expected login to fail, got nil")
	}
}
