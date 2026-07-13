package client

import (
	"bufio"
	"fmt"
	"strings"

	"maunium.net/go/mautrix"
)

const uiaKeyType = "type"

func handleUIA(c *Client, uiResp *mautrix.RespUserInteractive) any {
	hasPasswordFlow := false
	var fallbackStage mautrix.AuthType

	for _, flow := range uiResp.Flows {
		for _, stage := range flow.Stages {
			switch stage {
			case mautrix.AuthTypePassword:
				hasPasswordFlow = true
			case "m.oauth", "org.matrix.cross_signing_reset", mautrix.AuthTypeSSO:
				if fallbackStage == "" {
					fallbackStage = stage
				}
			}
		}
	}

	if fallbackStage != "" {
		return handleFallbackStage(c, fallbackStage, uiResp.Session)
	}

	if hasPasswordFlow {
		return handlePasswordFlow(c, uiResp.Session)
	}

	if _, err := fmt.Fprintf(stderr, "Error: The server requested UIA flows that are not supported by this client.\n"); err != nil {
		return nil
	}
	return nil
}

func handleFallbackStage(c *Client, fallbackStage mautrix.AuthType, session string) any {
	fallbackURL := fmt.Sprintf("%s/_matrix/client/v3/auth/%s/fallback/web?session=%s",
		strings.TrimSuffix(c.Matrix.HomeserverURL.String(), "/"),
		fallbackStage,
		session)

	if _, err := fmt.Fprintln(stdout, "\nThe server requires interactive authentication to proceed."); err != nil {
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "Please open the following link in your browser to confirm this action:\n\n%s\n\n", fallbackURL); err != nil {
		return nil
	}
	if _, err := fmt.Fprint(stdout, "Press Enter here once you have successfully completed the authentication in your browser..."); err != nil {
		return nil
	}

	reader := bufio.NewReader(stdin)
	if _, err := reader.ReadString('\n'); err != nil {
		return nil
	}

	return map[string]any{
		uiaKeyType: fallbackStage,
		"session":  session,
	}
}

func handlePasswordFlow(c *Client, session string) any {
	if _, err := fmt.Fprintln(stdout, "\nPassword confirmation is required for legacy servers."); err != nil {
		return nil
	}
	password, err := readPassword("Enter password: ")
	if err != nil {
		if _, printErr := fmt.Fprintf(stderr, "failed to read password: %v\n", err); printErr != nil {
			return nil
		}
		return nil
	}

	return map[string]any{
		uiaKeyType: mautrix.AuthTypePassword,
		"identifier": map[string]any{
			uiaKeyType: "m.id.user",
			"user":     c.Matrix.UserID.String(),
		},
		"password": password,
		"session":  session,
	}
}
