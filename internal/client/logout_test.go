package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/underhax/matrix-cli/internal/config"
)

func TestLogoutSession(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/logout") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("{}")); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
		}))
		defer srv.Close()

		session := &config.Session{
			HomeserverURL: srv.URL,
			UserID:        "@test1:example.com",
			DeviceID:      "DEV1",
			AccessToken:   "token1",
		}

		err := LogoutSession(context.Background(), session)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
	})

	t.Run("logout_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		session := &config.Session{
			HomeserverURL: srv.URL,
			UserID:        "@test2:example.net",
			DeviceID:      "DEV2",
			AccessToken:   "token2",
		}

		err := LogoutSession(context.Background(), session)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("new_client_error", func(t *testing.T) {
		session := &config.Session{
			HomeserverURL: "://invalid-url",
			UserID:        "@test3:example.org",
			DeviceID:      "DEV3",
			AccessToken:   "token3",
		}

		err := LogoutSession(context.Background(), session)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
