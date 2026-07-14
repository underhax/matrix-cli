package client

import (
	"bufio"
	"context"
	"fmt"
	"os"
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
	if c.Crypto == nil || c.Crypto.Machine() == nil {
		return
	}
	mach := c.Crypto.Machine()
	if sqlStore, ok := mach.CryptoStore.(*crypto.SQLCryptoStore); ok {
		if _, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_cross_signing_keys WHERE user_id=$1", userID); err != nil {
			c.Log.Debug().Err(err).Msg("failed to clean old keys")
		}
		if _, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_devices WHERE user_id=$1", userID); err != nil {
			c.Log.Debug().Err(err).Msg("failed to clean old devices")
		}
		if _, err := sqlStore.DB.Exec(ctx, "DELETE FROM crypto_cross_signing_signatures WHERE user_id=$1 OR sign_user_id=$1", userID); err != nil {
			c.Log.Debug().Err(err).Msg("failed to clean old signatures")
		}
	}
	if _, err := mach.FetchKeys(ctx, []id.UserID{userID}, true); err != nil {
		c.Log.Debug().Err(err).Msg("failed to fetch keys")
	}
}

// Verify blocks until the sync loop terminates, surfacing cancellation or transport errors
// raised by SyncWithContext. It is used to drive interactive device-verification flows that
// rely on the long-poll context to deliver incoming verification requests.
func (c *Client) Verify(ctx context.Context, targetUser string) error {
	if targetUser == "" {
		_, _ = fmt.Fprintln(os.Stderr, "Waiting for verification requests. Trigger verification from another device...")
		if err := c.Matrix.SyncWithContext(ctx); err != nil {
			return fmt.Errorf("verification sync aborted: %w", err)
		}
		return nil
	}

	userID := id.UserID(targetUser)
	_, _ = fmt.Fprintf(os.Stderr, "Initiating verification with %s...\n", userID)

	c.refreshCrossSigningKeys(ctx, userID)

	txnID, err := c.VH.StartVerification(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to start verification: %w", err)
	}
	_, _ = fmt.Fprintf(os.Stderr, "Started transaction %s. Waiting for the other side to accept...\n", txnID)

	if err := c.Matrix.SyncWithContext(ctx); err != nil {
		return fmt.Errorf("verification sync aborted: %w", err)
	}

	return nil
}

// VerificationRequested handles incoming verification requests by auto-accepting the transaction
// to enable headless SAS flow without manual initiation.
func (h *VerificationHandler) VerificationRequested(ctx context.Context, txnID id.VerificationTransactionID, from id.UserID, fromDevice id.DeviceID) {
	_, _ = fmt.Fprintf(os.Stderr, "\nIncoming verification request from %s (%s).\nAuto-accepting transaction %s...\n", from, fromDevice, txnID)
	go func() {
		if err := h.client.VH.AcceptVerification(ctx, txnID); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to accept verification: %v\n", err)
		}
	}()
}

// VerificationReady is invoked when both sides have acknowledged the verification transaction
// and initiates the SAS key agreement when supported.
func (h *VerificationHandler) VerificationReady(ctx context.Context, txnID id.VerificationTransactionID, _ id.DeviceID, supportsSAS, _ bool, _ *verificationhelper.QRCode) {
	if supportsSAS {
		_, _ = fmt.Fprintf(os.Stderr, "Verification ready. Starting SAS flow for %s...\n", txnID)
		go func() {
			if err := h.client.VH.StartSAS(ctx, txnID); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "StartSAS failed for %s: %v\n", txnID, err)
			}
		}()
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Verification ready, but SAS is not supported by the other device for %s.\n", txnID)
	}
}

// VerificationCancelled is invoked when either side aborts the verification exchange
// and logs the cancellation reason for operator diagnostics.
func (h *VerificationHandler) VerificationCancelled(_ context.Context, txnID id.VerificationTransactionID, code event.VerificationCancelCode, reason string) {
	_, _ = fmt.Fprintf(os.Stderr, "\nVerification %s cancelled: %s (%s)\n", txnID, reason, code)
}

// VerificationDone is invoked upon successful completion of the SAS key verification exchange.
func (h *VerificationHandler) VerificationDone(ctx context.Context, txnID id.VerificationTransactionID, _ event.VerificationMethod) {
	h.client.Log.Debug().Str("txn_id", string(txnID)).Msg("Verification done successfully!")
	bgCtx := context.WithoutCancel(ctx)
	h.client.requestSecrets(bgCtx)
}

// ShowSAS presents the emoji comparison challenge to the operator via stderr/stdin
// and confirms or rejects the SAS exchange based on interactive terminal input.
func (h *VerificationHandler) ShowSAS(ctx context.Context, txnID id.VerificationTransactionID, emojis []rune, emojiDescriptions []string, _ []int) {
	go func() {
		_, _ = fmt.Fprintf(os.Stderr, "\nCompare the following emojis with the other device:\n")
		for i, e := range emojis {
			desc := ""
			if i < len(emojiDescriptions) {
				desc = emojiDescriptions[i]
			}
			_, _ = fmt.Fprintf(os.Stderr, "%c - %s\n", e, desc)
		}
		_, _ = fmt.Fprintf(os.Stderr, "Do they match? (y/n): ")

		reader := bufio.NewReader(os.Stdin)

		input, readErr := reader.ReadString('\n')
		if readErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to read stdin: %v\n", readErr)
			return
		}

		if strings.ToLower(strings.TrimSpace(input)) == "y" {
			if err := h.client.VH.ConfirmSAS(ctx, txnID); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Failed to confirm SAS: %v\n", err)
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Verification confirmed locally. Waiting for other device to confirm...\n")
			}
		} else {
			if err := h.client.VH.CancelVerification(ctx, txnID, event.VerificationCancelCodeUser, "Mismatched emojis"); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Failed to cancel verification: %v\n", err)
			}
			_, _ = fmt.Fprintf(os.Stderr, "Verification aborted by user.\n")
		}
	}()
}
