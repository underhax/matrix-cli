package client

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

type uiaTestCase struct {
	name           string
	session        string
	stdinInput     string
	mockPassword   string
	customStdout   io.Writer
	customStderr   io.Writer
	customStdin    io.Reader
	mockPassErr    error
	expectedType   mautrix.AuthType
	flows          []mautrix.UIAFlow
	expectNil      bool
	expectErrPrint bool
}

type errorWriter struct {
	failOnWriteNum int
	writeCount     int
}

func (ew *errorWriter) Write(p []byte) (n int, err error) {
	ew.writeCount++
	if ew.writeCount == ew.failOnWriteNum {
		return 0, errors.New("write error")
	}
	return len(p), nil
}

type errorReader struct{}

func (er errorReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestHandleUIA(t *testing.T) {
	origStdout := stdout
	origStderr := stderr
	origStdin := stdin
	origReadPassword := readPassword
	defer func() {
		stdout = origStdout
		stderr = origStderr
		stdin = origStdin
		readPassword = origReadPassword
	}()

	hsURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	mockClient := &Client{
		Matrix: &mautrix.Client{
			HomeserverURL: hsURL,
			UserID:        id.UserID("@user:example.com"),
		},
	}

	tests := []uiaTestCase{
		{
			name: "fallback_oauth_success",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypeSSO}},
			},
			session:      "sess_123",
			stdinInput:   "\n",
			expectedType: mautrix.AuthTypeSSO,
		},
		{
			name: "fallback_fprintln_err",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypeSSO}},
			},
			customStdout: &errorWriter{failOnWriteNum: 1},
			expectNil:    true,
		},
		{
			name: "fallback_fprintf_err",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypeSSO}},
			},
			customStdout: &errorWriter{failOnWriteNum: 2},
			expectNil:    true,
		},
		{
			name: "fallback_fprint_err",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypeSSO}},
			},
			customStdout: &errorWriter{failOnWriteNum: 3},
			expectNil:    true,
		},
		{
			name: "fallback_stdin_err",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypeSSO}},
			},
			customStdin: errorReader{},
			expectNil:   true,
		},
		{
			name: "password_flow_success",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypePassword}},
			},
			session:      "sess_456",
			mockPassword: "my_password",
			expectedType: mautrix.AuthTypePassword,
		},
		{
			name: "password_fprintln_err",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypePassword}},
			},
			customStdout: &errorWriter{failOnWriteNum: 1},
			expectNil:    true,
		},
		{
			name: "password_flow_error",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypePassword}},
			},
			session:        "sess_789",
			mockPassErr:    errors.New("mock read error"),
			expectNil:      true,
			expectErrPrint: true,
		},
		{
			name: "password_read_and_stderr_err",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{mautrix.AuthTypePassword}},
			},
			session:      "sess_789",
			mockPassErr:  errors.New("mock read error"),
			customStderr: &errorWriter{failOnWriteNum: 1},
			expectNil:    true,
		},
		{
			name: "unsupported_flow",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{"m.login.dummy"}},
			},
			session:        "sess_000",
			expectNil:      true,
			expectErrPrint: true,
		},
		{
			name: "unsupported_flow_err_print",
			flows: []mautrix.UIAFlow{
				{Stages: []mautrix.AuthType{"m.login.dummy"}},
			},
			customStderr: &errorWriter{failOnWriteNum: 1},
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outBuf := new(bytes.Buffer)
			errBuf := new(bytes.Buffer)
			inBuf := bytes.NewBufferString(tt.stdinInput)

			if tt.customStdout != nil {
				stdout = tt.customStdout
			} else {
				stdout = outBuf
			}

			if tt.customStderr != nil {
				stderr = tt.customStderr
			} else {
				stderr = errBuf
			}

			if tt.customStdin != nil {
				stdin = tt.customStdin
			} else {
				stdin = inBuf
			}

			readPassword = func(_ string) (string, error) {
				return tt.mockPassword, tt.mockPassErr
			}

			uiResp := &mautrix.RespUserInteractive{
				Flows:   tt.flows,
				Session: tt.session,
			}

			res := handleUIA(mockClient, uiResp)

			verifyTestHandleUIAResult(t, &tt, res, errBuf)
		})
	}
}

func verifyTestHandleUIAResult(t *testing.T, tt *uiaTestCase, res any, errBuf *bytes.Buffer) {
	if tt.expectNil {
		if res != nil {
			t.Errorf("expected nil result, got %v", res)
		}
		if tt.expectErrPrint && tt.customStderr == nil && errBuf.Len() == 0 {
			t.Errorf("expected output to stderr, but got nothing")
		}
		return
	}

	resMap, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	if resMap[uiaKeyType] != tt.expectedType {
		t.Errorf("expected type %s, got %v", tt.expectedType, resMap[uiaKeyType])
	}
	if resMap["session"] != tt.session {
		t.Errorf("expected session %s, got %v", tt.session, resMap["session"])
	}
	if tt.expectedType == mautrix.AuthTypePassword {
		if resMap["password"] != tt.mockPassword {
			t.Errorf("expected password %s, got %v", tt.mockPassword, resMap["password"])
		}
	}

	if tt.expectErrPrint && tt.customStderr == nil && errBuf.Len() == 0 {
		t.Errorf("expected output to stderr, but got nothing")
	}
}
