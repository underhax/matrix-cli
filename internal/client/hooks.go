package client

import (
	"context"
	"fmt"
	"os"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func (c *Client) registerStateHooks() {
	syncer, ok := c.Matrix.Syncer.(mautrix.ExtensibleSyncer)
	if !ok {
		return
	}

	syncer.OnEventType(event.StateMember, c.onStateMember)
	syncer.OnEventType(event.StateEncryption, c.onStateEncryption)
	syncer.OnEventType(event.ToDeviceSecretRequest, c.onSecretRequest)
}

var secretRequestDelay = 2 * time.Second

func (c *Client) onSecretRequest(ctx context.Context, evt *event.Event) {
	req, ok := evt.Content.Parsed.(*event.SecretRequestEventContent)
	if !ok {
		return
	}
	mach := getCryptoMachine(c.Crypto)
	if mach == nil {
		return
	}

	if req.Action == event.SecretRequestCancellation {
		c.Log.Debug().Str("sender", string(evt.Sender)).Str("target_device_id", string(req.RequestingDeviceID)).Str("request_id", req.RequestID).Msg("Received secret request cancellation")
		return
	}
	c.Log.Debug().Str("sender", string(evt.Sender)).Str("target_device_id", string(req.RequestingDeviceID)).Str("request_id", req.RequestID).Str("secret", string(req.Name)).Msg("Received secret request")

	bgCtx := context.WithoutCancel(ctx)
	go func() {
		time.Sleep(secretRequestDelay)
		c.Log.Debug().Str("sender", string(evt.Sender)).Str("target_device_id", string(req.RequestingDeviceID)).Str("secret", string(req.Name)).Msg("Processing request after delay")
		doDebugHandleSecretRequest(bgCtx, c, mach, evt.Sender, req)
	}()
}

var doDebugHandleSecretRequest = defaultDebugHandleSecretRequest

func defaultDebugHandleSecretRequest(ctx context.Context, c *Client, mach *crypto.OlmMachine, userID id.UserID, content *event.SecretRequestEventContent) {
	if content.Action != event.SecretRequestRequest {
		return
	}

	ownDeviceID := string(mach.Client.DeviceID)
	targetDeviceID := string(content.RequestingDeviceID)

	if userID != mach.Client.UserID || targetDeviceID == "" {
		c.Log.Debug().Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Ignored: not from own device or empty device ID")
		return
	}
	if targetDeviceID == ownDeviceID {
		c.Log.Debug().Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Ignored: request from this device")
		return
	}

	device, err := getOrFetchDevice(ctx, mach, userID, content.RequestingDeviceID)
	if err != nil {
		c.Log.Debug().Err(err).Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Failed to fetch device")
		return
	}
	trust, err := resolveTrustContext(ctx, mach, device)
	if err != nil {
		c.Log.Debug().Err(err).Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Failed to resolve trust")
		return
	}
	if trust < id.TrustStateCrossSignedVerified {
		c.Log.Debug().Int("trust", int(trust)).Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Device is not verified, ignoring")
		return
	}

	secret, err := getSecret(ctx, mach, content.Name)
	if err != nil {
		c.Log.Debug().Err(err).Str("secret", string(content.Name)).Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Failed to get secret from store")
		return
	} else if secret == "" {
		c.Log.Debug().Str("secret", string(content.Name)).Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Secret is empty in store")
		return
	}

	c.Log.Debug().
		Str("to", targetDeviceID).
		Str("from", ownDeviceID).
		Str("secret", string(content.Name)).
		Msg("SENDING secret to device")
	err = sendEncryptedToDevice(ctx, mach, device, event.ToDeviceSecretSend, event.Content{
		Parsed: &event.SecretSendEventContent{
			RequestID: content.RequestID,
			Secret:    secret,
		},
	})
	if err != nil {
		c.Log.Debug().Err(err).Str("own_device_id", ownDeviceID).Str("target_device_id", targetDeviceID).Msg("Failed to send encrypted secret")
	}
}

func (c *Client) onStateEncryption(ctx context.Context, evt *event.Event) {
	if encContent, ok := evt.Content.Parsed.(*event.EncryptionEventContent); ok {
		if err := c.Matrix.StateStore.SetEncryptionEvent(ctx, evt.RoomID, encContent); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to update encryption state for %s: %v\n", evt.RoomID, err)
		}
	}
}

func (c *Client) onStateMember(ctx context.Context, evt *event.Event) {
	if err := c.Matrix.StateStore.SetMembership(ctx, evt.RoomID, id.UserID(evt.GetStateKey()), evt.Content.AsMember().Membership); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to update membership state for %s: %v\n", evt.GetStateKey(), err)
	}
}
