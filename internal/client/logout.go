package client

import (
	"context"
	"fmt"

	"github.com/underhax/matrix-cli/internal/config"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// LogoutSession invalidates the access token on the homeserver and removes the device.
// It bypasses crypto initialization to ensure it works even if device keys are corrupted.
func LogoutSession(ctx context.Context, session *config.Session) error {
	cli, err := mautrix.NewClient(session.HomeserverURL, id.UserID(session.UserID), session.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to create raw client: %w", err)
	}
	cli.DeviceID = id.DeviceID(session.DeviceID)

	_, err = cli.Logout(ctx)
	if err != nil {
		return fmt.Errorf("homeserver logout rejected: %w", err)
	}
	return nil
}
