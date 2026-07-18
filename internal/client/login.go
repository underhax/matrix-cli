package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/underhax/matrix-cli/internal/config"

	"maunium.net/go/mautrix"
)

func resolveHomeserver(ctx context.Context, server string) string {
	if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
		return server
	}

	wellKnown, err := discoverClientAPI(ctx, server)
	if err == nil && wellKnown != nil && wellKnown.Homeserver.BaseURL != "" {
		return wellKnown.Homeserver.BaseURL
	}

	return "https://" + server
}

// Login performs smart authentication against the homeserver, choosing SSO/OAuth if available,
// or falling back to interactive password prompts. It returns the populated session.
func Login(ctx context.Context, server, user, pass, deviceName, ssoCallbackPort string) (*config.Session, error) {
	resolved := resolveHomeserver(ctx, server)

	cli, err := mautrixNewClient(resolved, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to init pre-login client: %w", err)
	}

	if pass != "" {
		if user == "" {
			return nil, errors.New("--user is required when passing --pass explicitly")
		}
		return performPasswordLogin(ctx, cli, user, pass, deviceName, resolved)
	}

	flowsResp, err := cli.GetLoginFlows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch login flows: %w", err)
	}

	supportsSSO := false
	supportsPassword := false

	for _, flow := range flowsResp.Flows {
		switch flow.Type {
		case mautrix.AuthTypeSSO, "m.login.oauth2":
			supportsSSO = true
		case mautrix.AuthTypePassword:
			supportsPassword = true
		}
	}

	if supportsSSO {
		return performSSOLogin(ctx, cli, resolved, ssoCallbackPort, deviceName)
	}

	if supportsPassword {
		var promptErr error
		user, pass, promptErr = ensureUserAndPass(user, pass)
		if promptErr != nil {
			return nil, promptErr
		}
		return performPasswordLogin(ctx, cli, user, pass, deviceName, resolved)
	}

	return nil, errors.New("the server supports no compatible login flows (neither SSO nor password)")
}

func ensureUserAndPass(user, pass string) (finalUser, finalPass string, err error) {
	if user == "" {
		if _, err := fmt.Fprint(stdout, "Enter Matrix username: "); err != nil {
			return "", "", fmt.Errorf("failed to prompt for username: %w", err)
		}
		reader := bufio.NewReader(stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read username: %w", err)
		}
		user = strings.TrimSpace(input)
	}
	if pass == "" {
		var pwdErr error
		pass, pwdErr = ReadPassword("Enter password: ")
		if pwdErr != nil {
			return "", "", fmt.Errorf("failed to read password: %w", pwdErr)
		}
	}
	return user, pass, nil
}

type loginTokenHandler struct {
	tokenChan chan<- string
	errChan   chan<- error
}

func (h loginTokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("loginToken")
	if token == "" {
		w.WriteHeader(http.StatusBadRequest)
		if _, wErr := w.Write([]byte("Error: Missing loginToken in callback")); wErr != nil {
			h.errChan <- fmt.Errorf("failed to write error response: %w", wErr)
			return
		}
		h.errChan <- errors.New("missing loginToken in SSO callback")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	htmlResp := `<!DOCTYPE html><html><head><link rel="icon" href="data:,"></head><body>Authentication successful. You can safely close this window.</body></html>`
	if _, wErr := w.Write([]byte(htmlResp)); wErr != nil {
		h.errChan <- fmt.Errorf("failed to write success response: %w", wErr)
		return
	}
	h.tokenChan <- token
}

func serveSSOCallback(srv *http.Server, listener net.Listener, errChan chan<- error) {
	if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		errChan <- serveErr
	}
}

func startCallbackServer(ctx context.Context, ssoCallbackPort string, tokenChan chan<- string, errChan chan<- error) (*http.Server, int, error) {
	var listener net.Listener
	var err error

	if ssoCallbackPort != "" {
		listener, err = listenContext(ctx, "tcp", "127.0.0.1:"+ssoCallbackPort)
	} else {
		ports := []string{"8080", "8443", "8008", "0"}
		for _, port := range ports {
			listener, err = listenContext(ctx, "tcp", "127.0.0.1:"+port)
			if err == nil {
				break
			}
		}
	}

	if err != nil {
		return nil, 0, fmt.Errorf("failed to start local callback server: %w", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return nil, 0, errors.New("failed to cast listener address to TCPAddr")
	}
	port := addr.Port

	srv := &http.Server{
		Handler: loginTokenHandler{
			tokenChan: tokenChan,
			errChan:   errChan,
		},
		ReadHeaderTimeout: 5 * time.Second,
	}
	go serveSSOCallback(srv, listener, errChan)

	return srv, port, nil
}

func printSSOInstructions(serverURL, redirectURL string) error {
	ssoURL := fmt.Sprintf("%s/_matrix/client/v3/login/sso/redirect?redirectUrl=%s", strings.TrimSuffix(serverURL, "/"), redirectURL)

	if _, printErr := fmt.Fprintln(stdout, "\nThe server supports SSO/OAuth authentication."); printErr != nil {
		return fmt.Errorf("failed to print sso instructions: %w", printErr)
	}
	if _, printErr := fmt.Fprintf(stdout, "Please open the following link in your browser to log in:\n\n%s\n\n", ssoURL); printErr != nil {
		return fmt.Errorf("failed to print sso url: %w", printErr)
	}
	if _, printErr := fmt.Fprintln(stdout, "Waiting for browser callback..."); printErr != nil {
		return fmt.Errorf("failed to print waiting message: %w", printErr)
	}
	return nil
}

var performSSOLogin = defaultPerformSSOLogin

func defaultPerformSSOLogin(ctx context.Context, cli *mautrix.Client, serverURL, ssoCallbackPort, deviceName string) (*config.Session, error) {
	tokenChan := make(chan string, 1)
	errChan := make(chan error, 1)

	srv, port, err := startCallbackServer(ctx, ssoCallbackPort, tokenChan, errChan)
	if err != nil {
		return nil, err
	}

	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	if printErr := printSSOInstructions(serverURL, redirectURL); printErr != nil {
		if shutErr := srv.Shutdown(ctx); shutErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to shutdown server: %v\n", shutErr)
		}
		return nil, printErr
	}

	var loginToken string
	select {
	case <-ctx.Done():
		if shutErr := srv.Shutdown(ctx); shutErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to shutdown server: %v\n", shutErr)
		}
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	case cbErr := <-errChan:
		if shutErr := srv.Shutdown(ctx); shutErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to shutdown server: %v\n", shutErr)
		}
		return nil, fmt.Errorf("SSO callback error: %w", cbErr)
	case loginToken = <-tokenChan:
		if shutErr := srv.Shutdown(ctx); shutErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to shutdown server: %v\n", shutErr)
		}
	}

	req := &mautrix.ReqLogin{
		Type:                     mautrix.AuthTypeToken,
		Token:                    loginToken,
		InitialDeviceDisplayName: deviceName,
	}

	resp, loginErr := cli.Login(ctx, req)
	if loginErr != nil {
		return nil, fmt.Errorf("token login request failed: %w", loginErr)
	}

	return &config.Session{
		HomeserverURL: serverURL,
		UserID:        string(resp.UserID),
		AccessToken:   resp.AccessToken,
		DeviceID:      string(resp.DeviceID),
		DeviceName:    deviceName,
	}, nil
}

func performPasswordLogin(ctx context.Context, cli *mautrix.Client, user, pass, deviceName, serverURL string) (*config.Session, error) {
	req := &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: user,
		},
		Password:                 pass,
		InitialDeviceDisplayName: deviceName,
	}

	resp, err := cli.Login(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}

	return &config.Session{
		HomeserverURL: serverURL,
		UserID:        string(resp.UserID),
		AccessToken:   resp.AccessToken,
		DeviceID:      string(resp.DeviceID),
		DeviceName:    deviceName,
	}, nil
}
