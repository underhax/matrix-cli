package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/underhax/matrix-cli/internal/config"
	"github.com/underhax/matrix-cli/internal/consts"
)

func mockDiscoverSuccess(_ context.Context, server string) (*mautrix.ClientWellKnown, error) {
	if server != "example.org" {
		return nil, errors.New("unexpected server")
	}
	return &mautrix.ClientWellKnown{
		Homeserver: mautrix.HomeserverInfo{
			BaseURL: "https://matrix.example.org",
		},
	}, nil
}

func mockDiscoverFail(_ context.Context, _ string) (*mautrix.ClientWellKnown, error) {
	return nil, errors.New("not found")
}

func TestResolveHomeserver(t *testing.T) {
	origDiscover := discoverClientAPI
	defer func() { discoverClientAPI = origDiscover }()

	tests := []struct {
		name     string
		server   string
		mockFunc func(context.Context, string) (*mautrix.ClientWellKnown, error)
		expected string
	}{
		{
			name:     "with_http",
			server:   "http://resolve.example.com",
			expected: "http://resolve.example.com",
		},
		{
			name:     "with_https",
			server:   "https://resolve.example.net",
			expected: "https://resolve.example.net",
		},
		{
			name:     "without_http_discover_success",
			server:   "example.org",
			mockFunc: mockDiscoverSuccess,
			expected: "https://matrix.example.org",
		},
		{
			name:     "without_http_discover_fail",
			server:   "example.net",
			mockFunc: mockDiscoverFail,
			expected: "https://example.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockFunc != nil {
				discoverClientAPI = tt.mockFunc
			}
			res := resolveHomeserver(context.Background(), tt.server)
			if res != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, res)
			}
		})
	}
}

type loginErrorWriter struct{}

func (w loginErrorWriter) Write(_ []byte) (n int, err error) {
	time.Sleep(50 * time.Millisecond)
	return 0, errors.New("write error")
}

type loginErrorReader struct{}

func (loginErrorReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func mockPerformSSOLoginErr(_ context.Context, _ *mautrix.Client, _, _, _ string) (*config.Session, error) {
	return nil, errors.New("mock SSO error")
}

func mockPerformSSOLoginSuccess(_ context.Context, _ *mautrix.Client, _, _, _ string) (*config.Session, error) {
	return &config.Session{UserID: "@sso:example.net"}, nil
}

func mockMautrixNewClientErr(_ string, _ id.UserID, _ string) (*mautrix.Client, error) {
	return nil, errors.New("mock new client err")
}

func mockTermIsTerminalSuccess(_ int) bool {
	return true
}

func mockTermReadPasswordErr(_ int) ([]byte, error) {
	return nil, errors.New("mock read password err")
}

func mockTermReadPasswordSuccess(_ int) ([]byte, error) {
	return []byte("mypassword"), nil
}

func TestLogin_Flows(t *testing.T) {
	origMautrixNewClient := mautrixNewClient
	origStdout := stdout
	origStdin := stdin
	origTermReadPassword := termReadPassword
	origTermIsTerminal := termIsTerminal
	origPerformSSOLogin := performSSOLogin
	defer func() {
		mautrixNewClient = origMautrixNewClient
		stdout = origStdout
		stdin = origStdin
		termReadPassword = origTermReadPassword
		termIsTerminal = origTermIsTerminal
		performSSOLogin = origPerformSSOLogin
	}()

	stdout = io.Discard
	termIsTerminal = mockTermIsTerminalSuccess

	tests := []struct {
		flowsResp       *mautrix.RespLoginFlows
		name            string
		pass            string
		user            string
		mockStdin       string
		errContains     string
		expectedUser    string
		newClientErr    bool
		mockReadPassErr bool
		mockStdinErr    bool
		mockStdoutErr   bool
		mockSSOErr      bool
		mockSSOSuccess  bool
		flowsErr        bool
		loginErr        bool
		cancelCtx       bool
		expectedErr     bool
	}{
		{
			name:         "new_client_err",
			newClientErr: true,
			expectedErr:  true,
			errContains:  "pre-login client",
		},
		{
			name:        "pass_explicit_no_user",
			pass:        "pass1",
			expectedErr: true,
			errContains: "--user is required",
		},
		{
			name:         "pass_explicit_success",
			pass:         "pass2",
			user:         "user2",
			expectedErr:  false,
			expectedUser: "@server:example.com",
		},
		{
			name:        "pass_explicit_failure",
			pass:        "pass3",
			user:        "user3",
			loginErr:    true,
			expectedErr: true,
			errContains: "HTTP 403",
		},
		{
			name:        "get_flows_err",
			flowsErr:    true,
			expectedErr: true,
			errContains: "fetch login flows",
		},
		{
			name: "no_compatible_flows",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: "m.login.dummy_test_flow"}},
			},
			expectedErr: true,
			errContains: "no compatible login flows",
		},
		{
			name: "sso_flow_cancel",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypeSSO}},
			},
			cancelCtx:   true,
			expectedErr: true,
			errContains: "context canceled",
		},
		{
			name: "sso_flow_mock_err",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypeSSO}},
			},
			mockSSOErr:  true,
			expectedErr: true,
			errContains: "mock SSO error",
		},
		{
			name: "sso_flow_mock_success",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypeSSO}},
			},
			mockSSOSuccess: true,
			expectedErr:    false,
			expectedUser:   "@sso:example.net",
		},
		{
			name: "password_flow_success_stdin",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypePassword}},
			},
			mockStdin:   "myuser\n",
			expectedErr: false,
		},
		{
			name: "password_flow_read_pass_err",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypePassword}},
			},
			user:            "myuser",
			mockReadPassErr: true,
			expectedErr:     true,
			errContains:     "read password",
		},
		{
			name: "password_flow_stdout_err_user",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypePassword}},
			},
			mockStdoutErr: true,
			expectedErr:   true,
			errContains:   "prompt for username",
		},
		{
			name: "password_flow_stdin_err_user",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypePassword}},
			},
			mockStdinErr: true,
			expectedErr:  true,
			errContains:  "read username",
		},
		{
			name: "password_flow_stdout_err_pass",
			flowsResp: &mautrix.RespLoginFlows{
				Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypePassword}},
			},
			user:          "myuser",
			mockStdoutErr: true,
			expectedErr:   true,
			errContains:   "failed to print prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.newClientErr {
				mautrixNewClient = mockMautrixNewClientErr
			} else {
				mautrixNewClient = origMautrixNewClient
			}

			if tt.mockReadPassErr {
				termReadPassword = mockTermReadPasswordErr
			} else {
				termReadPassword = mockTermReadPasswordSuccess
			}

			switch {
			case tt.mockSSOErr:
				performSSOLogin = mockPerformSSOLoginErr
			case tt.mockSSOSuccess:
				performSSOLogin = mockPerformSSOLoginSuccess
			default:
				performSSOLogin = origPerformSSOLogin
			}

			switch {
			case tt.mockStdinErr:
				stdin = loginErrorReader{}
			case tt.mockStdin != "":
				r, w := io.Pipe()
				go func() {
					if _, err := w.Write([]byte(tt.mockStdin)); err != nil {
						panic(err)
					}
					if err := w.Close(); err != nil {
						panic(err)
					}
				}()
				stdin = r
			default:
				stdin = strings.NewReader("")
			}

			if tt.mockStdoutErr {
				stdout = loginErrorWriter{}
			} else {
				stdout = io.Discard
			}

			srv := createMockLoginServer(tt.flowsErr, tt.loginErr, tt.flowsResp)
			defer srv.Close()

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.cancelCtx {
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			session, err := Login(ctx, srv.URL, tt.user, tt.pass, "TestDevice", "")
			verifyLoginResult(t, session, err, tt.expectedErr, tt.errContains, tt.expectedUser)
		})
	}
}

func createMockLoginServer(flowsErr, loginErr bool, flowsResp *mautrix.RespLoginFlows) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_matrix/client/r0/login" && r.URL.Path != "/_matrix/client/v3/login" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			if flowsErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(flowsResp); err != nil {
				panic(err)
			}
			return
		}
		if r.Method == http.MethodPost {
			if loginErr {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(map[string]any{
				consts.KeyUserID:      "@server:example.com",
				consts.KeyAccessToken: "token_flows_1",
				consts.KeyDeviceID:    "DEV1",
			}); err != nil {
				panic(err)
			}
			return
		}
	}))
}

func verifyLoginResult(t *testing.T, session *config.Session, err error, expectedErr bool, errContains, expectedUser string) {
	t.Helper()
	if expectedErr {
		if err == nil {
			t.Errorf("expected error containing %q, got nil", errContains)
		} else if errContains != "" && !strings.Contains(err.Error(), errContains) {
			t.Errorf("expected error containing %q, got %v", errContains, err)
		}
		return
	}

	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
	if session == nil {
		t.Errorf("expected session, got nil")
	} else if expectedUser != "" && session.UserID != expectedUser {
		t.Errorf("expected user ID %s, got %s", expectedUser, session.UserID)
	}
}

type mockResponseWriter struct {
	writeErr error
	status   int
}

func (m *mockResponseWriter) Header() http.Header         { return http.Header{} }
func (m *mockResponseWriter) Write(_ []byte) (int, error) { return 0, m.writeErr }
func (m *mockResponseWriter) WriteHeader(statusCode int)  { m.status = statusCode }

func TestLoginTokenHandler(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		writeErr      error
		expectedErr   string
		expectedToken string
		expectedCode  int
	}{
		{
			name:         "missing_token",
			token:        "",
			expectedErr:  "missing loginToken in SSO callback",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "missing_token_write_err",
			token:        "",
			writeErr:     errors.New("mock write err"),
			expectedErr:  "failed to write error response: mock write err",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:          "success_flow",
			token:         "test_token_123",
			expectedToken: "test_token_123",
			expectedCode:  http.StatusOK,
		},
		{
			name:         "success_flow_write_err",
			token:        "test_token_456",
			writeErr:     errors.New("mock write err 2"),
			expectedErr:  "failed to write success response: mock write err 2",
			expectedCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenChan := make(chan string, 1)
			errChan := make(chan error, 1)

			handler := loginTokenHandler{
				tokenChan: tokenChan,
				errChan:   errChan,
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/callback?loginToken="+tt.token, http.NoBody)
			if err != nil {
				t.Fatalf("failed to create req: %v", err)
			}

			var rw http.ResponseWriter
			mrw := &mockResponseWriter{writeErr: tt.writeErr}
			rw = mrw

			if tt.writeErr == nil {
				rw = httptest.NewRecorder()
			}

			handler.ServeHTTP(rw, req)

			if tt.writeErr != nil {
				if mrw.status != tt.expectedCode {
					t.Errorf("expected status %d, got %d", tt.expectedCode, mrw.status)
				}
			} else {
				rec, ok := rw.(*httptest.ResponseRecorder)
				if !ok {
					t.Fatalf("expected ResponseRecorder")
				}
				if rec.Code != tt.expectedCode {
					t.Errorf("expected status %d, got %d", tt.expectedCode, rec.Code)
				}
			}

			if tt.expectedErr != "" {
				select {
				case err := <-errChan:
					if err.Error() != tt.expectedErr {
						t.Errorf("expected err %q, got %q", tt.expectedErr, err.Error())
					}
				default:
					t.Errorf("expected error %q, got none", tt.expectedErr)
				}
			} else {
				select {
				case <-errChan:
					t.Errorf("expected no error")
				default:
				}
			}

			if tt.expectedToken != "" {
				select {
				case tok := <-tokenChan:
					if tok != tt.expectedToken {
						t.Errorf("expected token %q, got %q", tt.expectedToken, tok)
					}
				default:
					t.Errorf("expected token %q, got none", tt.expectedToken)
				}
			}
		})
	}
}

type mockListener struct {
	acceptErr error
	addrFunc  func() net.Addr
}

func (m *mockListener) Accept() (net.Conn, error) {
	return nil, m.acceptErr
}
func (m *mockListener) Close() error {
	return nil
}
func (m *mockListener) Addr() net.Addr {
	if m.addrFunc != nil {
		return m.addrFunc()
	}
	return &net.TCPAddr{Port: 1234}
}

func TestServeSSOCallback(t *testing.T) {
	errChan := make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second}

	ml := &mockListener{acceptErr: http.ErrServerClosed}
	serveSSOCallback(srv, ml, errChan)

	select {
	case <-errChan:
		t.Errorf("expected no error for ErrServerClosed")
	default:
	}

	expectedErr := errors.New("mock accept err")
	ml.acceptErr = expectedErr
	serveSSOCallback(srv, ml, errChan)

	select {
	case err := <-errChan:
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	default:
		t.Errorf("expected error, got none")
	}
}

func mockListenContextSuccess(_ context.Context, _, _ string) (net.Listener, error) {
	return &mockListener{}, nil
}

func mockListenContextErr(_ context.Context, _, _ string) (net.Listener, error) {
	return nil, errors.New("mock listen error")
}

func TestStartCallbackServer(t *testing.T) {
	origListenContext := listenContext
	defer func() { listenContext = origListenContext }()

	tokenChan := make(chan string, 1)
	errChan := make(chan error, 1)

	listenContext = mockListenContextSuccess
	srv, port, err := startCallbackServer(context.Background(), "0", tokenChan, errChan)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if port != 1234 {
		t.Errorf("expected mock port 1234, got %d", port)
	}
	if closeErr := srv.Close(); closeErr != nil {
		t.Errorf("failed to close srv: %v", closeErr)
	}

	listenContext = func(_ context.Context, _, address string) (net.Listener, error) {
		if strings.HasSuffix(address, ":8080") {
			return nil, errors.New("port in use")
		}
		return &mockListener{}, nil
	}
	srv, port, err = startCallbackServer(context.Background(), "", tokenChan, errChan)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if port != 1234 {
		t.Errorf("expected mock port 1234, got %d", port)
	}
	if closeErr := srv.Close(); closeErr != nil {
		t.Errorf("failed to close srv: %v", closeErr)
	}

	listenContext = func(_ context.Context, _, _ string) (net.Listener, error) {
		ml := &mockListener{}
		ml.addrFunc = func() net.Addr { return &net.UnixAddr{} }
		return ml, nil
	}
	_, _, err = startCallbackServer(context.Background(), "0", tokenChan, errChan)
	if err == nil || !strings.Contains(err.Error(), "failed to cast listener address") {
		t.Fatalf("expected cast error, got %v", err)
	}

	listenContext = mockListenContextErr
	_, _, err = startCallbackServer(context.Background(), "-1", tokenChan, errChan)
	if err == nil {
		t.Fatalf("expected error for explicit listen failure")
	}

	_, _, err = startCallbackServer(context.Background(), "", tokenChan, errChan)
	if err == nil {
		t.Fatalf("expected error for fallback listen failure")
	}
}

func TestDefaultListenContext(t *testing.T) {
	listener, err := defaultListenContext(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Errorf("failed to close listener: %v", err)
	}
}

type loginFailOnPatternWriter struct {
	failOn string
}

func (w *loginFailOnPatternWriter) Write(p []byte) (n int, err error) {
	if strings.Contains(string(p), w.failOn) {
		return 0, errors.New("pattern write error")
	}
	return len(p), nil
}

func TestPrintSSOInstructions(t *testing.T) {
	origStdout := stdout
	defer func() { stdout = origStdout }()

	var buf strings.Builder
	stdout = &buf
	err := printSSOInstructions("https://example.com/", "http://localhost:8080/callback")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "https://example.com/_matrix/client/v3/login/sso/redirect?redirectUrl=http://localhost:8080/callback") {
		t.Errorf("missing expected url in output: %s", out)
	}

	stdout = &loginFailOnPatternWriter{failOn: "SSO/OAuth authentication"}
	err = printSSOInstructions("https://example.com", "http://localhost:8080/callback")
	if err == nil || !strings.Contains(err.Error(), "failed to print sso instructions") {
		t.Fatalf("expected sso instructions print error, got %v", err)
	}

	stdout = &loginFailOnPatternWriter{failOn: "open the following link"}
	err = printSSOInstructions("https://example.com", "http://localhost:8080/callback")
	if err == nil || !strings.Contains(err.Error(), "failed to print sso url") {
		t.Fatalf("expected sso url print error, got %v", err)
	}

	stdout = &loginFailOnPatternWriter{failOn: "Waiting for browser callback"}
	err = printSSOInstructions("https://example.com", "http://localhost:8080/callback")
	if err == nil || !strings.Contains(err.Error(), "failed to print waiting message") {
		t.Fatalf("expected waiting message print error, got %v", err)
	}
}

func setupSSOMockServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "login") {
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req["token"] == "valid_token" {
				if encErr := json.NewEncoder(w).Encode(map[string]any{
					"user_id":      "@test:example.com",
					"access_token": "token123",
					"device_id":    "DEV123",
				}); encErr != nil {
					t := &testing.T{}
					t.Logf("failed to encode: %v", encErr)
				}
				return
			}
			w.WriteHeader(http.StatusForbidden)
			if _, writeErr := w.Write([]byte(`{"errcode": "M_FORBIDDEN"}`)); writeErr != nil {
				t := &testing.T{}
				t.Logf("failed to write: %v", writeErr)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

type mockErrorCloseListener struct {
	net.Listener
}

func (m *mockErrorCloseListener) Close() error {
	if err := m.Listener.Close(); err != nil {
		return err
	}
	return errors.New("mock close error")
}

func mockSSOListenContext(orig func(context.Context, string, string) (net.Listener, error), portChan chan<- int) func(context.Context, string, string) (net.Listener, error) {
	return func(ctx context.Context, n, _ string) (net.Listener, error) {
		l, err := orig(ctx, n, "127.0.0.1:0")
		if err == nil {
			if tcpAddr, ok := l.Addr().(*net.TCPAddr); ok && portChan != nil {
				portChan <- tcpAddr.Port
			}
			return &mockErrorCloseListener{Listener: l}, nil
		}
		return l, err
	}
}

func doSSOGet(ctx context.Context, port int, path string) error {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), http.NoBody)
	if reqErr != nil {
		return reqErr
	}
	resp, doErr := http.DefaultClient.Do(req)
	if doErr == nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return closeErr
		}
	}
	return doErr
}

func TestDefaultPerformSSOLogin_Failures(t *testing.T) {
	origStdout := stdout
	origStderr := stderr
	origListenContext := listenContext
	defer func() {
		stdout = origStdout
		stderr = origStderr
		listenContext = origListenContext
	}()

	stdout = &strings.Builder{}
	stderr = &strings.Builder{}

	mockMatrixSrv := setupSSOMockServer()
	defer mockMatrixSrv.Close()

	cli, cliErr := mautrix.NewClient(mockMatrixSrv.URL, "", "")
	if cliErr != nil {
		t.Fatalf("failed to create client: %v", cliErr)
	}

	t.Run("server_start_fail", func(t *testing.T) {
		listenContext = mockListenContextErr
		_, err := defaultPerformSSOLogin(context.Background(), cli, mockMatrixSrv.URL, "0", "TestDev")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("print_instructions_fail", func(t *testing.T) {
		listenContext = mockSSOListenContext(origListenContext, nil)
		stdout = loginErrorWriter{}
		_, err := defaultPerformSSOLogin(context.Background(), cli, mockMatrixSrv.URL, "0", "TestDev")
		if err == nil || !strings.Contains(err.Error(), "failed to print sso instructions") {
			t.Fatalf("expected print err, got %v", err)
		}
	})

	t.Run("context_cancel", func(t *testing.T) {
		stdout = &strings.Builder{}
		listenContext = mockSSOListenContext(origListenContext, nil)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		_, err := defaultPerformSSOLogin(ctx, cli, mockMatrixSrv.URL, "0", "TestDev")
		if err == nil || !strings.Contains(err.Error(), "context cancelled") {
			t.Fatalf("expected cancel err, got %v", err)
		}
	})
}

func TestDefaultPerformSSOLogin_Network(t *testing.T) {
	origStdout := stdout
	origStderr := stderr
	origListenContext := listenContext
	defer func() {
		stdout = origStdout
		stderr = origStderr
		listenContext = origListenContext
	}()

	stdout = &strings.Builder{}
	stderr = &strings.Builder{}

	mockMatrixSrv := setupSSOMockServer()
	defer mockMatrixSrv.Close()

	cli, cliErr := mautrix.NewClient(mockMatrixSrv.URL, "", "")
	if cliErr != nil {
		t.Fatalf("failed to create client: %v", cliErr)
	}

	t.Run("callback_error", func(t *testing.T) {
		stdout = &strings.Builder{}
		portChan := make(chan int, 1)
		listenContext = mockSSOListenContext(origListenContext, portChan)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := defaultPerformSSOLogin(ctx, cli, mockMatrixSrv.URL, "0", "TestDev")
			done <- err
		}()

		port := <-portChan
		if err := doSSOGet(ctx, port, "/callback"); err != nil {
			t.Logf("get failed: %v", err)
		}

		err := <-done
		if err == nil || !strings.Contains(err.Error(), "SSO callback error") {
			t.Fatalf("expected callback error, got %v", err)
		}
	})

	t.Run("login_api_fail", func(t *testing.T) {
		stdout = &strings.Builder{}
		portChan := make(chan int, 1)
		listenContext = mockSSOListenContext(origListenContext, portChan)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := defaultPerformSSOLogin(ctx, cli, mockMatrixSrv.URL, "0", "TestDev")
			done <- err
		}()

		port := <-portChan
		if err := doSSOGet(ctx, port, "/callback?loginToken=bad_token"); err != nil {
			t.Logf("get failed: %v", err)
		}

		err := <-done
		if err == nil || !strings.Contains(err.Error(), "token login request failed") {
			t.Fatalf("expected login api error, got %v", err)
		}
	})

	t.Run("login_success", func(t *testing.T) {
		stdout = &strings.Builder{}
		portChan := make(chan int, 1)
		listenContext = mockSSOListenContext(origListenContext, portChan)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		type result struct {
			s   *config.Session
			err error
		}
		done := make(chan result, 1)

		go func() {
			s, err := defaultPerformSSOLogin(ctx, cli, mockMatrixSrv.URL, "0", "TestDev")
			done <- result{s, err}
		}()

		port := <-portChan
		if err := doSSOGet(ctx, port, "/callback?loginToken=valid_token"); err != nil {
			t.Logf("get failed: %v", err)
		}

		res := <-done
		if res.err != nil {
			t.Fatalf("expected success, got %v", res.err)
		}
		if res.s.AccessToken != "token123" {
			t.Errorf("expected token123, got %s", res.s.AccessToken)
		}
	})
}
