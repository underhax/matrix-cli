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
			message:           "hello 1",
			stateEventStatus:  500,
			expectErr:         true,
			expectErrContains: "failed to fetch room encryption state",
		},
		{
			name:              "state_store_encryption_err",
			roomID:            "!room2:example.com",
			message:           "hello 2",
			stateEventStatus:  200,
			setEncryptionErr:  errors.New("mock set encryption error"),
			expectErr:         true,
			expectErrContains: "failed to store room encryption state",
		},
		{
			name:              "joined_members_err",
			roomID:            "!room3:example.com",
			message:           "hello 3",
			stateEventStatus:  200,
			joinedMemStatus:   500,
			expectErr:         true,
			expectErrContains: "failed to fetch room members",
		},
		{
			name:              "state_store_membership_err",
			roomID:            "!room4:example.com",
			message:           "hello 4",
			stateEventStatus:  200,
			joinedMemStatus:   200,
			setMembershipErr:  errors.New("mock set membership error"),
			expectErr:         true,
			expectErrContains: "failed to populate state store membership",
		},
		{
			name:              "send_message_err",
			roomID:            "!room5:example.com",
			message:           "hello 5",
			stateEventStatus:  404,
			sendMsgStatus:     500,
			expectErr:         true,
			expectErrContains: "failed to transmit event",
		},
		{
			name:             "success_unencrypted",
			roomID:           "!room6:example.com",
			message:          "hello 6",
			stateEventStatus: 404,
			sendMsgStatus:    200,
			expectErr:        false,
		},
		{
			name:             "success_encrypted",
			roomID:           "!room7:example.com",
			message:          "hello 7",
			stateEventStatus: 200,
			joinedMemStatus:  200,
			sendMsgStatus:    200,
			expectErr:        false,
		},
		{
			name:             "success_html",
			roomID:           "!room_html:example.com",
			message:          "<b>hello html</b>",
			stateEventStatus: 404,
			sendMsgStatus:    200,
			expectErr:        false,
			isHTML:           true,
		},
		{
			name:             "success_markdown",
			roomID:           "!room_md:example.com",
			message:          "**hello markdown**",
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
}

func TestSend(t *testing.T) {
	tests := []sendTestCase{
		{
			name:              "no_rooms",
			roomsStr:          "",
			message:           "hello 8",
			expectErr:         true,
			expectErrContains: "no rooms specified",
		},
		{
			name:         "success",
			roomsStr:     "!room8:example.com",
			message:      "hello 9",
			expectErr:    false,
			expectStdout: `"status":"success"`,
		},
		{
			name:          "send_to_room_err",
			roomsStr:      "!room9:example.com !room10:example.com",
			message:       "hello 10",
			sendToRoomErr: errors.New("mock send error"),
			expectErr:     false,
			expectStdout:  `"status":"error"`,
		},
		{
			name:              "stdout_print_err_stderr_fails",
			roomsStr:          "!room11:example.com",
			message:           "hello 11",
			stdoutErrNum:      1,
			stderrErrNum:      1,
			expectErr:         true,
			expectErrContains: "failed to output result",
		},
		{
			name:              "stdout_print_err_stderr_ok",
			roomsStr:          "!room11:example.com",
			message:           "hello 11",
			stdoutErrNum:      1,
			stderrErrNum:      0,
			expectErr:         true,
			expectErrContains: "failed to output result",
		},
		{
			name:           "json_marshal_err",
			roomsStr:       "!room12:example.com",
			message:        "hello 12",
			jsonMarshalErr: errors.New("mock json error"),
			expectErr:      false,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			origStdout := stdout
			origStderr := stderr
			defer func() {
				stdout = origStdout
				stderr = origStderr
			}()

			var outBuf bytes.Buffer
			if tt.stdoutErrNum > 0 {
				stdout = &errorWriter{failOnWriteNum: tt.stdoutErrNum}
			} else {
				stdout = &outBuf
			}

			var errBuf bytes.Buffer
			if tt.stderrErrNum > 0 {
				stderr = &errorWriter{failOnWriteNum: tt.stderrErrNum}
			} else {
				stderr = &errBuf
			}

			if tt.jsonMarshalErr != nil {
				origJSON := jsonMarshal
				defer func() { jsonMarshal = origJSON }()
				jsonMarshal = func(_ any) ([]byte, error) {
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
