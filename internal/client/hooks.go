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

func (c *Client) onSecretRequest(ctx context.Context, evt *event.Event) {
	if req, ok := evt.Content.Parsed.(*event.SecretRequestEventContent); ok {
		if mach := c.Crypto.Machine(); mach != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Received secret request from %s for %s\n", evt.Sender, req.Name)
			bgCtx := context.WithoutCancel(ctx)
			go func() {
				time.Sleep(2 * time.Second)
				if DebugMode {
					_, _ = fmt.Fprintf(os.Stderr, "[DEBUG SECRET] Processing request %s from %s after delay...\n", req.Name, evt.Sender)
				}
				debugHandleSecretRequest(bgCtx, mach, evt.Sender, req)
			}()
		}
	}
}

func debugHandleSecretRequest(ctx context.Context, mach *crypto.OlmMachine, userID id.UserID, content *event.SecretRequestEventContent) {
	if content.Action != event.SecretRequestRequest {
		return
	}

	debugLog := func(format string, args ...any) {
		if DebugMode {
			_, _ = fmt.Fprintf(os.Stderr, format, args...)
		}
	}

	if userID != mach.Client.UserID || content.RequestingDeviceID == "" {
		debugLog("[DEBUG SECRET] Ignored: not from own device or empty device ID\n")
		return
	}
	if content.RequestingDeviceID == mach.Client.DeviceID {
		debugLog("[DEBUG SECRET] Ignored: request from this device\n")
		return
	}

	device, err := mach.GetOrFetchDevice(ctx, mach.Client.UserID, content.RequestingDeviceID)
	if err != nil {
		debugLog("[DEBUG SECRET] Failed to fetch device %s: %v\n", content.RequestingDeviceID, err)
		return
	}
	trust, err := mach.ResolveTrustContext(ctx, device)
	if err != nil {
		debugLog("[DEBUG SECRET] Failed to resolve trust for %s: %v\n", content.RequestingDeviceID, err)
		return
	}
	if trust < id.TrustStateCrossSignedVerified {
		debugLog("[DEBUG SECRET] Device %s is not verified (trust=%d), ignoring\n", content.RequestingDeviceID, trust)
		return
	}

	secret, err := mach.CryptoStore.GetSecret(ctx, content.Name)
	if err != nil {
		debugLog("[DEBUG SECRET] Failed to get secret %s from store: %v\n", content.Name, err)
		return
	} else if secret == "" {
		debugLog("[DEBUG SECRET] Secret %s is empty in store\n", content.Name)
		return
	}

	debugLog("[DEBUG SECRET] SENDING secret %s to device %s\n", content.Name, content.RequestingDeviceID)
	err = mach.SendEncryptedToDevice(ctx, device, event.ToDeviceSecretSend, event.Content{
		Parsed: event.SecretSendEventContent{
			RequestID: content.RequestID,
			Secret:    secret,
		},
	})
	if err != nil {
		debugLog("[DEBUG SECRET] Failed to send encrypted secret: %v\n", err)
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
