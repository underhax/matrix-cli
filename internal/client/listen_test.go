package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type listenTestCase struct {
	jsonMarshalErr    error
	syncHit           chan struct{}
	name              string
	roomsStr          string
	expectErrContains string
	stderrFailMsg     string
	stdoutFailMsg     string
	eventID           string
	stopSync          bool
	expectErr         bool
}

type listenMockWriter struct {
	failContains string
}

func (w *listenMockWriter) Write(p []byte) (n int, err error) {
	if w.failContains != "" && strings.Contains(string(p), w.failContains) {
		return 0, errors.New("mock write err")
	}
	return len(p), nil
}

func setupListenMockServer(t *testing.T, tt *listenTestCase) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/sync") {
			if tt.syncHit != nil {
				select {
				case tt.syncHit <- struct{}{}:
				default:
				}
			}
			if !tt.stopSync {
				time.Sleep(100 * time.Millisecond)
				writeMockResp(t, w, 401, `{"errcode":"M_UNKNOWN_TOKEN","error":"sync failed"}`)
				return
			}
			writeMockResp(t, w, 200, `{"next_batch": "batch_listen_1"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/filter") {
			writeMockResp(t, w, 200, `{"filter_id": "mock_filter_id"}`)
			return
		}

		writeMockResp(t, w, 404, `{"errcode":"M_UNRECOGNIZED","error":"unrecognized endpoint"}`)
	}))
}

func TestListen(t *testing.T) {
	tests := []listenTestCase{
		{
			name:              "stderr_write_err_start",
			stderrFailMsg:     "starting infinite sync loop",
			expectErr:         true,
			expectErrContains: "failed to write to stderr",
		},
		{
			name:              "filter_ignores_room",
			roomsStr:          "!listen_other_room:example.com",
			expectErr:         true,
			expectErrContains: "loop terminated",
		},
		{
			name:              "marshal_err_stderr_ok",
			roomsStr:          "!listen_r2:example.com",
			jsonMarshalErr:    errors.New("mock listen marshal err 1"),
			expectErr:         true,
			expectErrContains: "sync loop term",
		},
		{
			name:              "marshal_err_stderr_err",
			roomsStr:          "!listen_r3:example.com",
			jsonMarshalErr:    errors.New("mock listen marshal err 2"),
			stderrFailMsg:     "failed to marshal event",
			expectErr:         true,
			expectErrContains: "terminated",
		},
		{
			name:              "stdout_err_stderr_ok",
			roomsStr:          "!listen_r4:example.com",
			stdoutFailMsg:     "$listen_evt_1",
			eventID:           "$listen_evt_1",
			expectErr:         true,
			expectErrContains: "loop term",
		},
		{
			name:              "stdout_err_stderr_err",
			roomsStr:          "!listen_r5:example.com",
			stdoutFailMsg:     "$listen_evt_2",
			eventID:           "$listen_evt_2",
			stderrFailMsg:     "stdout write error",
			expectErr:         true,
			expectErrContains: "c loop term",
		},
		{
			name:      "listen_success",
			roomsStr:  "!listen_r6:example.com",
			stopSync:  true,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runListenTest(t, &tt)
		})
	}
}

func runListenTest(t *testing.T, tt *listenTestCase) {
	tt.syncHit = make(chan struct{}, 1)
	server := setupListenMockServer(t, tt)
	defer server.Close()

	matrixClient, err := mautrix.NewClient(server.URL, "@user:example.com", "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	matrixClient.Syncer = mautrix.NewDefaultSyncer()
	memStore := mautrix.NewMemorySyncStore()
	if saveErr := memStore.SaveNextBatch(context.Background(), "@user:example.com", "dummy_token"); saveErr != nil {
		t.Fatalf("failed to save next batch: %v", saveErr)
	}
	matrixClient.Store = memStore
	c := &Client{Matrix: matrixClient}

	origJSON := jsonMarshal
	origStdout := stdout
	origStderr := stderr
	defer func() {
		jsonMarshal = origJSON
		stdout = origStdout
		stderr = origStderr
	}()
	setupListenMockIO(tt)

	listenErr := make(chan error, 1)
	go func() {
		listenErr <- c.Listen(context.Background(), tt.roomsStr)
	}()

	select {
	case <-tt.syncHit:
		dispatchListenEvent(matrixClient, tt)
		err = <-listenErr
	case err = <-listenErr:
	}

	switch {
	case tt.expectErr && err == nil:
		t.Errorf("expected error containing %q, got nil", tt.expectErrContains)
	case !tt.expectErr && err != nil:
		t.Errorf("expected no error, got: %v", err)
	case tt.expectErr && err != nil && !strings.Contains(err.Error(), tt.expectErrContains):
		t.Errorf("expected error containing %q, got: %v", tt.expectErrContains, err)
	}
}

func dispatchListenEvent(matrixClient *mautrix.Client, tt *listenTestCase) {
	syncer, ok := matrixClient.Syncer.(*mautrix.DefaultSyncer)
	if ok {
		roomID := tt.roomsStr
		if roomID == "" || roomID == "!listen_other_room:example.com" {
			roomID = "!listen_mock_room:example.com"
		}
		evtID := tt.eventID
		if evtID == "" {
			evtID = "$listen_evt_def"
		}
		syncer.Dispatch(context.Background(), &event.Event{
			Type:   event.EventMessage,
			RoomID: id.RoomID(roomID),
			ID:     id.EventID(evtID),
		})
	}
	if tt.stopSync {
		matrixClient.StopSync()
	}
}

func setupListenMockIO(tt *listenTestCase) {
	if tt.jsonMarshalErr != nil {
		jsonMarshal = func(_ any) ([]byte, error) {
			return nil, tt.jsonMarshalErr
		}
	}

	if tt.stdoutFailMsg != "" {
		stdout = &listenMockWriter{failContains: tt.stdoutFailMsg}
	} else {
		stdout = io.Discard
	}

	if tt.stderrFailMsg != "" {
		stderr = &listenMockWriter{failContains: tt.stderrFailMsg}
	} else {
		stderr = io.Discard
	}
}

func TestListen_NotExtensibleSyncer(t *testing.T) {
	client := &Client{Matrix: &mautrix.Client{}}
	err := client.Listen(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "does not implement mautrix.ExtensibleSyncer") {
		t.Errorf("expected ExtensibleSyncer error, got: %v", err)
	}
}
