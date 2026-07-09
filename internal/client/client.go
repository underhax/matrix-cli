// Package client provides the Matrix client operations and E2EE state management.
package client

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"matrix-cli/internal/config"
)

// Client encapsulates the Matrix client, cryptographic state machine, and persistence layer
// to orchestrate E2EE operations in a headless environment.
type Client struct {
	Matrix *mautrix.Client
	Crypto *cryptohelper.CryptoHelper
	DB     *sql.DB
	VH     *verificationhelper.VerificationHelper
}

// VerificationHandler implements the required callbacks for SAS verification.
type VerificationHandler struct {
	client *Client
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
// and proceeds to the SAS key agreement when supported.
func (h *VerificationHandler) VerificationReady(ctx context.Context, txnID id.VerificationTransactionID, _ id.DeviceID, supportsSAS, _ bool, _ *verificationhelper.QRCode) {
	if supportsSAS {
		_, _ = fmt.Fprintf(os.Stderr, "Verification ready. Starting SAS flow for %s...\n", txnID)
		go func() {
			if err := h.client.VH.StartSAS(ctx, txnID); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Failed to start SAS: %v\n", err)
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
func (h *VerificationHandler) VerificationDone(_ context.Context, txnID id.VerificationTransactionID, _ event.VerificationMethod) {
	_, _ = fmt.Fprintf(os.Stderr, "\nVerification %s done successfully!\n", txnID)
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

// New initializes the mautrix client and delegates all cryptographic lifecycle management
// (store creation, table migrations, OlmMachine setup, syncer hooks) to cryptohelper.CryptoHelper.
func New(ctx context.Context, session *config.Session, db *sql.DB) (*Client, error) {
	cli, err := mautrix.NewClient(session.HomeserverURL, id.UserID(session.UserID), session.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create matrix client: %w", err)
	}
	cli.DeviceID = id.DeviceID(session.DeviceID)

	dbWrap, err := dbutil.NewWithDB(db, "sqlite3")
	if err != nil {
		return nil, fmt.Errorf("failed to wrap database: %w", err)
	}

	pickleKey := []byte("default_local_encryption_key_change_in_production")

	ch, err := cryptohelper.NewCryptoHelper(cli, pickleKey, dbWrap)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto helper: %w", err)
	}

	if err := ch.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to init crypto helper: %w", err)
	}
	cli.Crypto = ch

	mach := ch.Machine()

	clientObj := &Client{
		Matrix: cli,
		Crypto: ch,
		DB:     db,
	}

	handler := &VerificationHandler{client: clientObj}
	vh := verificationhelper.NewVerificationHelper(cli, mach, verificationhelper.NewInMemoryVerificationStore(), handler, false, false, true)
	if err := vh.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to init verification helper: %w", err)
	}
	clientObj.VH = vh

	return clientObj, nil
}

// Login performs a standard password-based authentication against the homeserver
// and populates the session payload for subsequent executions.
func Login(ctx context.Context, server, user, pass, deviceName string) (*config.Session, error) {
	cli, err := mautrix.NewClient(server, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to init pre-login client: %w", err)
	}

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
		HomeserverURL: server,
		UserID:        string(resp.UserID),
		AccessToken:   resp.AccessToken,
		DeviceID:      string(resp.DeviceID),
	}, nil
}

// Listen starts an infinite sync loop, decrypting E2EE events and piping them
// strictly to stdout as compact JSON to ensure parser compliance for downstream shell tools.
func (c *Client) Listen(_ context.Context) error {
	syncer, ok := c.Matrix.Syncer.(mautrix.ExtensibleSyncer)
	if !ok {
		return errors.New("syncer does not implement mautrix.ExtensibleSyncer")
	}

	syncer.OnEventType(event.EventMessage, func(_ context.Context, evt *event.Event) {
		payload, err := json.Marshal(evt)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to marshal event %s: %v\n", evt.ID, err)
			return
		}
		if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "stdout write error: %v\n", err)
		}
	})

	_, _ = fmt.Fprintln(os.Stderr, "starting infinite sync loop...")

	if err := c.Matrix.Sync(); err != nil {
		return fmt.Errorf("sync loop terminated: %w", err)
	}

	return nil
}

// Send fetches the room membership topology, populates the state store for key distribution,
// and dispatches the message through the CryptoHelper auto-encryption pipeline.
func (c *Client) Send(ctx context.Context, roomID, message string) error {
	parsedRoom := id.RoomID(roomID)

	var encEvt event.EncryptionEventContent
	if err := c.Matrix.StateEvent(ctx, parsedRoom, event.StateEncryption, "", &encEvt); err != nil {
		return fmt.Errorf("failed to fetch room encryption state: %w", err)
	}

	if err := c.Matrix.StateStore.SetEncryptionEvent(ctx, parsedRoom, &encEvt); err != nil {
		return fmt.Errorf("failed to store room encryption state: %w", err)
	}

	members, err := c.Matrix.JoinedMembers(ctx, parsedRoom)
	if err != nil {
		return fmt.Errorf("failed to fetch room members: %w", err)
	}

	for userID := range members.Joined {
		if setErr := c.Matrix.StateStore.SetMembership(ctx, parsedRoom, userID, event.MembershipJoin); setErr != nil {
			return fmt.Errorf("failed to populate state store membership for %s: %w", userID, setErr)
		}
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    message,
	}

	resp, err := c.Matrix.SendMessageEvent(ctx, parsedRoom, event.EventMessage, content)
	if err != nil {
		return fmt.Errorf("failed to transmit event: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "event sent successfully: %s\n", resp.EventID)
	return nil
}

// Verify initiates an interactive SAS terminal flow.
func (c *Client) Verify(_ context.Context) error {
	_, _ = fmt.Fprintln(os.Stderr, "Waiting for verification requests. Trigger verification from another device...")

	if err := c.Matrix.Sync(); err != nil {
		return fmt.Errorf("verification sync aborted: %w", err)
	}

	return nil
}
