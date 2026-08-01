package main

import (
	"reflect"
	"testing"

	"github.com/underhax/matrix-cli/internal/consts"
)

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		server     string
		user       string
		rooms      string
		session    string
		db         string
		pickle     string
		wantErrors []string
	}{
		{
			name:       "valid_all_inputs",
			mode:       consts.ModeAuth,
			server:     "https://example.com",
			user:       "@test:example.com",
			rooms:      "!room:example.com",
			session:    "nonexistent.json",
			db:         "nonexistent.db",
			pickle:     "nonexistent.key",
			wantErrors: nil,
		},
		{
			name:       "valid_domain_only",
			mode:       consts.ModeAuth,
			server:     "example.org",
			user:       "",
			rooms:      "",
			session:    "a.json",
			db:         "b.db",
			pickle:     "c.key",
			wantErrors: nil,
		},
		{
			name:       "invalid_server_url",
			mode:       consts.ModeAuth,
			server:     "http://[::1]:err",
			user:       "",
			rooms:      "",
			session:    "a",
			db:         "b",
			pickle:     "c",
			wantErrors: []string{`invalid URL format for server "http://[::1]:err"`},
		},
		{
			name:       "invalid_server_domain",
			mode:       consts.ModeAuth,
			server:     "invalid_domain",
			user:       "",
			rooms:      "",
			session:    "a",
			db:         "b",
			pickle:     "c",
			wantErrors: []string{`invalid domain structure for server "invalid_domain"`},
		},
		{
			name:       "invalid_user_id",
			mode:       consts.ModeVerify,
			server:     "",
			user:       "invalid_user",
			rooms:      "",
			session:    "a",
			db:         "b",
			pickle:     "c",
			wantErrors: []string{`invalid user ID format for user "invalid_user"`},
		},
		{
			name:    "invalid_room_ids",
			mode:    consts.ModeSend,
			server:  "",
			user:    "",
			rooms:   "invalid_room !valid:example.net bad_room",
			session: "a",
			db:      "b",
			pickle:  "c",
			wantErrors: []string{
				`invalid room ID format for room "invalid_room"`,
				`invalid room ID format for room "bad_room"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateInput(tt.mode, tt.server, tt.user, tt.rooms, tt.session, tt.db, tt.pickle)
			if len(got) == 0 && len(tt.wantErrors) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.wantErrors) {
				t.Errorf("validateInput()\ngot  = %v\nwant = %v", got, tt.wantErrors)
			}
		})
	}
}
