package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type sendMockStateStore struct {
	mautrix.StateStore
	setEncryptionErr error
	setMembershipErr error
}

func (m *sendMockStateStore) SetEncryptionEvent(_ context.Context, _ id.RoomID, _ *event.EncryptionEventContent) error {
	return m.setEncryptionErr
}

func (m *sendMockStateStore) SetMembership(_ context.Context, _ id.RoomID, _ id.UserID, _ event.Membership) error {
	return m.setMembershipErr
}

func (m *sendMockStateStore) ReplaceCachedMembers(_ context.Context, _ id.RoomID, _ []*event.Event, _ ...event.Membership) error {
	return nil
}

type sendToRoomTestCase struct {
	setEncryptionErr  error
	setMembershipErr  error
	name              string
	roomID            id.RoomID
	message           string
	expectErrContains string
	stateEventStatus  int
	joinedMemStatus   int
	sendMsgStatus     int
	expectErr         bool
	isHTML            bool
	isMarkdown        bool
}

func TestSendToRoom(t *testing.T) {
	tests := []sendToRoomTestCase{
		{
			name:              "state_event_error",
			roomID:            "!room1:example.com",
			message:           "Please review the latest deployment logs.",
			stateEventStatus:  500,
			expectErr:         true,
			expectErrContains: "failed to fetch room encryption state",
		},
		{
			name:              "state_store_encryption_err",
			roomID:            "!room2:example.com",
			message:           "Can we schedule a sync for tomorrow?",
			stateEventStatus:  200,
			setEncryptionErr:  errors.New("mock set encryption error"),
			expectErr:         true,
			expectErrContains: "failed to store room encryption state",
		},
		{
			name:              "joined_members_err",
			roomID:            "!room3:example.com",
			message:           "The database migration completed successfully.",
			stateEventStatus:  200,
			joinedMemStatus:   500,
			expectErr:         true,
			expectErrContains: "failed to fetch room members",
		},
		{
			name:              "state_store_membership_err",
			roomID:            "!room4:example.com",
			message:           "Reminder: Submit your timesheets by Friday.",
			stateEventStatus:  200,
			joinedMemStatus:   200,
			setMembershipErr:  errors.New("mock set membership error"),
			expectErr:         true,
			expectErrContains: "failed to populate state store membership",
		},
		{
			name:              "send_message_err",
			roomID:            "!room5:example.com",
			message:           "I encountered a bug in the staging environment.",
			stateEventStatus:  404,
			sendMsgStatus:     500,
			expectErr:         true,
			expectErrContains: "failed to transmit event",
		},
		{
			name:             "success_unencrypted",
			roomID:           "!room6:example.com",
			message:          "This is an unencrypted test message.",
			stateEventStatus: 404,
			sendMsgStatus:    200,
			expectErr:        false,
		},
		{
			name:             "success_encrypted",
			roomID:           "!room7:example.com",
			message:          "Secure transmission confirmed.",
			stateEventStatus: 200,
			joinedMemStatus:  200,
			sendMsgStatus:    200,
			expectErr:        false,
		},
		{
			name:             "success_html",
			roomID:           "!room_html:example.com",
			message:          "<b>Important update:</b> Server maintenance at midnight.",
			stateEventStatus: 404,
			sendMsgStatus:    200,
			expectErr:        false,
			isHTML:           true,
		},
		{
			name:             "success_markdown",
			roomID:           "!room_md:example.com",
			message:          "**Action Required:** Please approve the pull request.",
			stateEventStatus: 404,
			sendMsgStatus:    200,
			expectErr:        false,
			isMarkdown:       true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
					switch tt.stateEventStatus {
					case 200:
						writeMockResp(t, w, tt.stateEventStatus, `{"algorithm":"m.megolm.v1.aes-sha2"}`)
					case 500:
						writeMockResp(t, w, tt.stateEventStatus, `{"errcode":"M_UNKNOWN","error":"state error"}`)
					default:
						writeMockResp(t, w, tt.stateEventStatus, `{"errcode":"M_NOT_FOUND","error":"Event not found"}`)
					}
				case strings.Contains(r.URL.Path, "/joined_members"):
					if tt.joinedMemStatus == 200 {
						writeMockResp(t, w, tt.joinedMemStatus, `{"joined": {"@user:example.com": {}}}`)
					} else {
						writeMockResp(t, w, tt.joinedMemStatus, `{"errcode":"M_UNKNOWN","error":"join error"}`)
					}
				case strings.Contains(r.URL.Path, "/send/m.room.message"):
					if tt.sendMsgStatus == 200 {
						writeMockResp(t, w, tt.sendMsgStatus, `{"event_id":"$event1"}`)
					} else {
						writeMockResp(t, w, tt.sendMsgStatus, `{"errcode":"M_UNKNOWN","error":"send error"}`)
					}
				default:
					writeMockResp(t, w, 404, `{"errcode":"M_UNRECOGNIZED","error":"unrecognized endpoint"}`)
				}
			}))
			defer server.Close()

			matrixClient, err := mautrix.NewClient(server.URL, "@bot:example.com", "token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			store := &sendMockStateStore{
				setEncryptionErr: tt.setEncryptionErr,
				setMembershipErr: tt.setMembershipErr,
			}
			matrixClient.StateStore = store

			c := &Client{Matrix: matrixClient}
			eventID, err := c.sendToRoom(context.Background(), tt.roomID, tt.message, tt.isHTML, tt.isMarkdown)

			verifySendToRoomResult(t, &tt, eventID, err)
		})
	}
}

func writeMockResp(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Log("write err", err)
	}
}

func verifySendToRoomResult(t *testing.T, tt *sendToRoomTestCase, eventID string, err error) {
	switch {
	case tt.expectErr && err == nil:
		t.Fatalf("expected error containing %q, got nil", tt.expectErrContains)
	case tt.expectErr && !strings.Contains(err.Error(), tt.expectErrContains):
		t.Errorf("expected error containing %q, got %q", tt.expectErrContains, err.Error())
	case !tt.expectErr && err != nil:
		t.Fatalf("expected no error, got %v", err)
	case !tt.expectErr && eventID == "":
		t.Errorf("expected eventID, got empty string")
	}
}

type sendTestCase struct {
	sendToRoomErr     error
	jsonMarshalErr    error
	mockErr           error
	name              string
	roomsStr          string
	message           string
	expectErrContains string
	expectStdout      string
	stdoutErrNum      int
	stderrErrNum      int
	expectErr         bool
	isHTML            bool
	isMarkdown        bool
	jsonMode          bool
}

func TestSend(t *testing.T) {
	tests := []sendTestCase{
		{
			name:              "no_rooms",
			roomsStr:          "",
			message:           "Test broadcast to empty room list.",
			expectErr:         true,
			expectErrContains: "no rooms specified",
		},
		{
			name:         "success",
			roomsStr:     "!room8:example.com",
			message:      "System alert: high CPU usage detected.",
			expectErr:    false,
			expectStdout: `Successfully sent message to !room8:example.com (Event ID: $event1)`,
		},
		{
			name:         "success_json",
			roomsStr:     "!room8_json:example.com",
			message:      "System alert: memory usage critical.",
			expectErr:    false,
			jsonMode:     true,
			expectStdout: `"status": "success"`,
		},
		{
			name:          "send_to_room_err",
			roomsStr:      "!room9:example.com !room10:example.com",
			message:       "Failed delivery simulation.",
			sendToRoomErr: errors.New("mock send error"),
			expectErr:     false,
			expectStdout:  `Failed to send message to !room9:example.com: failed to fetch room encryption state: M_UNKNOWN (HTTP 500): mock send error`,
		},
		{
			name:              "send_to_room_err_stdout_fails",
			roomsStr:          "!room9_err:example.com",
			message:           "Failed delivery with stdout error.",
			sendToRoomErr:     errors.New("mock send error"),
			stdoutErrNum:      1,
			mockErr:           errors.New("failed to write error to stdout"),
			expectErr:         true,
			expectErrContains: "stdout write error: failed to write error to stdout",
		},
		{
			name:          "send_to_room_err_json",
			roomsStr:      "!room9_json:example.com",
			message:       "Failed delivery simulation in JSON.",
			sendToRoomErr: errors.New("mock send error"),
			expectErr:     false,
			jsonMode:      true,
			expectStdout:  `"status": "error"`,
		},
		{
			name:              "stdout_print_err_stderr_fails",
			roomsStr:          "!room11a:example.com",
			message:           "Testing JSON output failure with stderr.",
			jsonMode:          true,
			stdoutErrNum:      1,
			stderrErrNum:      1,
			mockErr:           errors.New("failed to write send json"),
			expectErr:         true,
			expectErrContains: "stdout write error: failed to write send json",
		},
		{
			name:              "stdout_print_err_stderr_ok",
			roomsStr:          "!room11b:example.com",
			message:           "Testing JSON output failure without stderr.",
			jsonMode:          true,
			stdoutErrNum:      1,
			stderrErrNum:      0,
			mockErr:           errors.New("failed to write send json (stderr ok)"),
			expectErr:         true,
			expectErrContains: "stdout write error: failed to write send json (stderr ok)",
		},
		{
			name:              "stdout_print_err_not_json",
			roomsStr:          "!room11c:example.com !room11d:example.com",
			message:           "Testing plain text output failure.",
			stdoutErrNum:      1,
			mockErr:           errors.New("failed to write send text"),
			expectErr:         true,
			expectErrContains: "stdout write error: failed to write send text",
		},
		{
			name:              "json_marshal_err",
			roomsStr:          "!room12:example.com",
			message:           "Testing JSON marshaling error handling.",
			jsonMode:          true,
			jsonMarshalErr:    errors.New("mock json error"),
			expectErr:         true,
			expectErrContains: "failed to marshal results: mock json error",
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			origStdout := stdout
			origStderr := stderr
			origJSONMode := JSONMode
			defer func() {
				stdout = origStdout
				stderr = origStderr
				JSONMode = origJSONMode
			}()
			JSONMode = tt.jsonMode

			var outBuf bytes.Buffer
			if tt.stdoutErrNum > 0 {
				stdout = &errorWriter{failOnWriteNum: tt.stdoutErrNum, mockErr: tt.mockErr}
			} else {
				stdout = &outBuf
			}

			var errBuf bytes.Buffer
			if tt.stderrErrNum > 0 {
				stderr = &errorWriter{failOnWriteNum: tt.stderrErrNum, mockErr: tt.mockErr}
			} else {
				stderr = &errBuf
			}

			if tt.jsonMarshalErr != nil {
				origJSON := jsonMarshalIndent
				defer func() { jsonMarshalIndent = origJSON }()
				jsonMarshalIndent = func(_ any, _, _ string) ([]byte, error) {
					return nil, tt.jsonMarshalErr
				}
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tt.sendToRoomErr != nil {
					writeMockResp(t, w, 500, `{"errcode":"M_UNKNOWN","error":"mock send error"}`)
					return
				}
				if strings.Contains(r.URL.Path, "/state/m.room.encryption") {
					writeMockResp(t, w, 404, `{"errcode":"M_NOT_FOUND","error":"Event not found"}`)
				} else if strings.Contains(r.URL.Path, "/send/m.room.message") {
					writeMockResp(t, w, 200, `{"event_id":"$event1"}`)
				}
			}))
			defer server.Close()

			matrixClient, err := mautrix.NewClient(server.URL, "@bot:example.com", "token")
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			c := &Client{Matrix: matrixClient}

			sendErr := c.Send(context.Background(), tt.roomsStr, tt.message, tt.isHTML, tt.isMarkdown)

			verifySendResult(t, &tt, sendErr, outBuf.String())
		})
	}
}

func verifySendResult(t *testing.T, tt *sendTestCase, err error, outStr string) {
	switch {
	case tt.expectErr && err == nil:
		t.Fatalf("expected error containing %q, got nil", tt.expectErrContains)
	case tt.expectErr && !strings.Contains(err.Error(), tt.expectErrContains):
		t.Errorf("expected error containing %q, got %q", tt.expectErrContains, err.Error())
	case !tt.expectErr && err != nil:
		t.Fatalf("expected no error, got %v", err)
	case !tt.expectErr && tt.expectStdout != "" && !strings.Contains(outStr, tt.expectStdout):
		t.Errorf("expected stdout containing %q, got %q", tt.expectStdout, outStr)
	}
}
