package client

import (
	"bufio"
	"fmt"
	"os"
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
		fallbackURL := fmt.Sprintf("%s/_matrix/client/v3/auth/%s/fallback/web?session=%s",
			strings.TrimSuffix(c.Matrix.HomeserverURL.String(), "/"),
			fallbackStage,
			uiResp.Session)

		if _, err := fmt.Fprintln(os.Stdout, "\nThe server requires interactive authentication to proceed."); err != nil {
			return nil
		}
		if _, err := fmt.Fprintf(os.Stdout, "Please open the following link in your browser to confirm this action:\n\n%s\n\n", fallbackURL); err != nil {
			return nil
		}
		if _, err := fmt.Fprint(os.Stdout, "Press Enter here once you have successfully completed the authentication in your browser..."); err != nil {
			return nil
		}

		reader := bufio.NewReader(os.Stdin)
		if _, err := reader.ReadString('\n'); err != nil {
			return nil
		}

		return map[string]any{
			uiaKeyType: fallbackStage,
			"session":  uiResp.Session,
		}
	}

	if hasPasswordFlow {
		if _, err := fmt.Fprintln(os.Stdout, "\nPassword confirmation is required for legacy servers."); err != nil {
			return nil
		}
		password, err := ReadPassword("Enter password: ")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to read password: %v\n", err)
			return nil
		}

		return map[string]any{
			uiaKeyType: mautrix.AuthTypePassword,
			"identifier": map[string]any{
				uiaKeyType: "m.id.user",
				"user":     c.Matrix.UserID.String(),
			},
			"password": password,
			"session":  uiResp.Session,
		}
	}

	if _, err := fmt.Fprintf(os.Stderr, "Error: The server requested UIA flows that are not supported by this client.\n"); err != nil {
		return nil
	}
	return nil
}
