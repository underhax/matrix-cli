package client

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// VerificationHandler implements the required callbacks for SAS verification.
type VerificationHandler struct {
	client *Client
}

func (c *Client) refreshCrossSigningKeys(ctx context.Context, userID id.UserID) {
	mach := getOlmMachine(c)
	if mach == nil {
		return
	}
	if err := clearCryptoCache(ctx, mach, userID); err != nil {
		c.Log.Debug().Err(err).Msg("failed to clean crypto cache")
	}
	if _, err := fetchKeys(ctx, mach, []id.UserID{userID}, true); err != nil {
		c.Log.Debug().Err(err).Msg("failed to fetch keys")
	}
	c.checkStalePrivateKeys(ctx, mach, userID)
}

func (c *Client) checkStalePrivateKeys(ctx context.Context, mach *crypto.OlmMachine, userID id.UserID) {
	if c.Matrix == nil || userID != c.Matrix.UserID || mach.CrossSigningKeys == nil || mach.CrossSigningKeys.MasterKey == nil {
		return
	}
	pubkeys, err := getCrossSigningPublicKeys(ctx, mach, userID)
	if err != nil || pubkeys == nil || pubkeys.MasterKey == "" {
		return
	}
	localPub := mach.CrossSigningKeys.PublicKeys()
	if pubkeys.MasterKey.String() != localPub.MasterKey.String() {
		c.Log.Debug().Msg("Local private cross-signing keys are stale, dropping them")
		mach.CrossSigningKeys = nil
		if err := clearCrossSigningSecrets(ctx, mach); err != nil {
			c.Log.Debug().Err(err).Msg("Failed to clear stale cross-signing secrets from db")
		}
	}
}

// Verify blocks until the sync loop terminates, surfacing cancellation or transport errors
// raised by SyncWithContext. It is used to drive interactive device-verification flows that
// rely on the long-poll context to deliver incoming verification requests.
func (c *Client) Verify(ctx context.Context, targetUser string) error {
	c.refreshCrossSigningKeys(ctx, c.Matrix.UserID)

	mach := getOlmMachine(c)
	if mach != nil {
		if pub, err := getOwnCrossSigningPublicKeys(ctx, mach); err == nil && pub != nil {
			mkStr := pub.MasterKey.String()
			if len(mkStr) > 8 {
				mkStr = mkStr[:8] + "..."
			}
			c.Log.Debug().
				Str("device_id", string(c.Matrix.DeviceID)).
				Str("master_key", mkStr).
				Msg("Current identity keys before verification")
		} else {
			c.Log.Debug().Err(err).Msg("Failed to get own master key or it is nil")
		}
	}

	if targetUser == "" {
		fprintlnStderr("Waiting for verification requests. Trigger verification from another device...")
		if err := matrixSyncWithContext(ctx, c.Matrix); err != nil {
			return fmt.Errorf("verification sync aborted: %w", err)
		}
		return nil
	}

	userID := id.UserID(targetUser)
	fprintfStderr("Initiating verification with %s...\n", userID)

	if userID != c.Matrix.UserID {
		c.refreshCrossSigningKeys(ctx, userID)
	}

	txnID, err := startVerification(ctx, c.VH, userID)
	if err != nil {
		return fmt.Errorf("failed to start verification: %w", err)
	}
	fprintfStderr("Started transaction %s. Waiting for the other side to accept...\n", txnID)

	if err := matrixSyncWithContext(ctx, c.Matrix); err != nil {
		return fmt.Errorf("verification sync aborted: %w", err)
	}

	return nil
}

// VerificationRequested handles incoming verification requests by auto-accepting the transaction
// to enable headless SAS flow without manual initiation.
func (h *VerificationHandler) VerificationRequested(ctx context.Context, txnID id.VerificationTransactionID, from id.UserID, fromDevice id.DeviceID) {
	fprintfStderr("\nIncoming verification request from %s (%s).\nAuto-accepting transaction %s...\n", from, fromDevice, txnID)
	h.client.refreshCrossSigningKeys(ctx, from)
	go func() {
		if err := acceptVerification(ctx, h.client.VH, txnID); err != nil {
			fprintfStderr("Failed to accept verification: %v\n", err)
		}
	}()
}

// VerificationReady is invoked when both sides have acknowledged the verification transaction
// and initiates the SAS key agreement when supported.
func (h *VerificationHandler) VerificationReady(ctx context.Context, txnID id.VerificationTransactionID, _ id.DeviceID, supportsSAS, _ bool, _ *verificationhelper.QRCode) {
	if supportsSAS {
		fprintfStderr("Verification ready. Starting SAS flow for %s...\n", txnID)
		go func() {
			if err := startSAS(ctx, h.client.VH, txnID); err != nil {
				fprintfStderr("StartSAS failed for %s: %v\n", txnID, err)
			}
		}()
	} else {
		fprintfStderr("Verification ready, but SAS is not supported by the other device for %s.\n", txnID)
	}
}

// VerificationCancelled is invoked when either side aborts the verification exchange
// and logs the cancellation reason for operator diagnostics.
func (h *VerificationHandler) VerificationCancelled(_ context.Context, txnID id.VerificationTransactionID, code event.VerificationCancelCode, reason string) {
	fprintfStderr("\nVerification %s cancelled: %s (%s)\n", txnID, reason, code)
}

// VerificationDone is invoked upon successful completion of the SAS key verification exchange.
func (h *VerificationHandler) VerificationDone(ctx context.Context, txnID id.VerificationTransactionID, _ event.VerificationMethod) {
	h.client.Log.Debug().Str("txn_id", string(txnID)).Msg("Verification done successfully!")
	bgCtx := context.WithoutCancel(ctx)
	h.client.requestSecrets(bgCtx, func() {
		fprintfStderr("\nVerification completed successfully. Press Ctrl+C to exit.\n")
	})
}

// ShowSAS presents the emoji comparison challenge to the operator via stderr/stdin
// and confirms or rejects the SAS exchange based on interactive terminal input.
func (h *VerificationHandler) ShowSAS(ctx context.Context, txnID id.VerificationTransactionID, emojis []rune, emojiDescriptions []string, _ []int) {
	go func() {
		fprintfStderr("\nCompare the following emojis with the other device:\n")
		for i, e := range emojis {
			desc := ""
			if i < len(emojiDescriptions) {
				desc = emojiDescriptions[i]
			}
			fprintfStderr("%c - %s\n", e, desc)
		}
		fprintfStderr("Do they match? (y/n): ")

		reader := bufio.NewReader(stdin)

		input, readErr := reader.ReadString('\n')
		if readErr != nil {
			fprintfStderr("Failed to read stdin: %v\n", readErr)
			return
		}

		if strings.ToLower(strings.TrimSpace(input)) == "y" {
			if err := confirmSAS(ctx, h.client.VH, txnID); err != nil {
				fprintfStderr("Failed to confirm SAS: %v\n", err)
			} else {
				fprintfStderr("Verification confirmed locally. Waiting for other device to confirm...\n")
			}
		} else {
			if err := cancelVerification(ctx, h.client.VH, txnID, event.VerificationCancelCodeUser, "Mismatched emojis"); err != nil {
				fprintfStderr("Failed to cancel verification: %v\n", err)
			}
			fprintfStderr("Verification aborted by user.\n")
		}
	}()
}
