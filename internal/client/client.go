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
	"sync/atomic"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/crypto/verificationhelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"matrix-cli/internal/config"
	"matrix-cli/internal/ui/spinner"
)

// RoomDetail represents the basic metadata of a Matrix room for JSON output.
type RoomDetail struct {
	RoomID         string `json:"room_id"`
	Name           string `json:"name,omitempty"`
	CanonicalAlias string `json:"canonical_alias,omitempty"`
	Topic          string `json:"topic,omitempty"`
}

// MemberInfo represents a room participant and their privileges.
type MemberInfo struct {
	UserID     string `json:"user_id"`
	Role       string `json:"role"`
	PowerLevel int    `json:"power_level"`
}

// DetailedRoomInfo represents extended metadata for a specific room.
type DetailedRoomInfo struct {
	RoomDetail
	Creator     string       `json:"creator,omitempty"`
	Version     string       `json:"version,omitempty"`
	Members     []MemberInfo `json:"members,omitempty"`
	MemberCount int          `json:"member_count"`
	Encrypted   bool         `json:"encrypted"`
}

// Client encapsulates the Matrix client, cryptographic state machine, and persistence layer
// to orchestrate E2EE operations in a headless environment.
type Client struct {
	Matrix *mautrix.Client
	Crypto *cryptohelper.CryptoHelper
	DB     *sql.DB
	VH     *verificationhelper.VerificationHelper
}

const (
	jsonKeyStatus   = "status"
	statusCancelled = "cancelled"
	statusSuccess   = "success"
)

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

	out := map[string]string{
		jsonKeyStatus: statusCancelled,
		"txn_id":      string(txnID),
		"reason":      reason,
		"code":        string(code),
	}
	if payload, err := json.Marshal(out); err == nil {
		if _, writeErr := fmt.Fprintln(os.Stdout, string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}
}

// VerificationDone is invoked upon successful completion of the SAS key verification exchange.
func (h *VerificationHandler) VerificationDone(_ context.Context, txnID id.VerificationTransactionID, _ event.VerificationMethod) {
	_, _ = fmt.Fprintf(os.Stderr, "\nVerification %s done successfully!\n", txnID)

	out := map[string]string{
		jsonKeyStatus: statusSuccess,
		"txn_id":      string(txnID),
	}
	if payload, err := json.Marshal(out); err == nil {
		if _, writeErr := fmt.Fprintln(os.Stdout, string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}
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

	clientObj.registerStateHooks()

	handler := &VerificationHandler{client: clientObj}
	vh := verificationhelper.NewVerificationHelper(cli, mach, verificationhelper.NewInMemoryVerificationStore(), handler, false, false, true)
	if err := vh.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to init verification helper: %w", err)
	}
	clientObj.VH = vh

	return clientObj, nil
}

func (c *Client) registerStateHooks() {
	syncer, ok := c.Matrix.Syncer.(mautrix.ExtensibleSyncer)
	if !ok {
		return
	}

	syncer.OnEventType(event.StateMember, c.onStateMember)
	syncer.OnEventType(event.StateEncryption, c.onStateEncryption)
}

func (c *Client) onStateMember(ctx context.Context, evt *event.Event) {
	if err := c.Matrix.StateStore.SetMembership(ctx, evt.RoomID, id.UserID(evt.GetStateKey()), evt.Content.AsMember().Membership); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to update membership state for %s: %v\n", evt.GetStateKey(), err)
	}
}

func (c *Client) onStateEncryption(ctx context.Context, evt *event.Event) {
	if encContent, ok := evt.Content.Parsed.(*event.EncryptionEventContent); ok {
		if err := c.Matrix.StateStore.SetEncryptionEvent(ctx, evt.RoomID, encContent); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to update encryption state for %s: %v\n", evt.RoomID, err)
		}
	}
}

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

func resolveHomeserver(ctx context.Context, server string) string {
	if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
		return server
	}

	wellKnown, err := mautrix.DiscoverClientAPI(ctx, server)
	if err == nil && wellKnown != nil && wellKnown.Homeserver.BaseURL != "" {
		return wellKnown.Homeserver.BaseURL
	}

	return "https://" + server
}

// Login performs a standard password-based authentication against the homeserver
// and populates the session payload for subsequent executions.
func Login(ctx context.Context, server, user, pass, deviceName string) (*config.Session, error) {
	resolved := resolveHomeserver(ctx, server)

	cli, err := mautrix.NewClient(resolved, "", "")
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
		HomeserverURL: resolved,
		UserID:        string(resp.UserID),
		AccessToken:   resp.AccessToken,
		DeviceID:      string(resp.DeviceID),
		DeviceName:    deviceName,
	}, nil
}

// Listen starts an infinite sync loop, decrypting E2EE events and piping them
// strictly to stdout as compact JSON to ensure parser compliance for downstream shell tools.
// If roomsStr is provided, it filters incoming events strictly to the specified space-separated room IDs.
func (c *Client) Listen(_ context.Context, roomsStr string) error {
	syncer, ok := c.Matrix.Syncer.(mautrix.ExtensibleSyncer)
	if !ok {
		return errors.New("syncer does not implement mautrix.ExtensibleSyncer")
	}

	allowedRooms := make(map[string]bool)
	for r := range strings.FieldsSeq(roomsStr) {
		allowedRooms[r] = true
	}

	syncer.OnEventType(event.EventMessage, func(_ context.Context, evt *event.Event) {
		if len(allowedRooms) > 0 && !allowedRooms[evt.RoomID.String()] {
			return
		}

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
// and dispatches the message through the CryptoHelper auto-encryption pipeline to multiple rooms.
func (c *Client) Send(ctx context.Context, roomsStr, message string) error {
	roomList := strings.Fields(roomsStr)
	if len(roomList) == 0 {
		return errors.New("no rooms specified")
	}

	var results []map[string]string

	for _, r := range roomList {
		parsedRoom := id.RoomID(r)
		res := map[string]string{
			"room_id": r,
		}

		eventID, err := c.sendToRoom(ctx, parsedRoom, message)
		if err != nil {
			res[jsonKeyStatus] = "error"
			res["error"] = err.Error()
		} else {
			res[jsonKeyStatus] = statusSuccess
			res["event_id"] = eventID
		}
		results = append(results, res)
	}

	if payload, err := json.Marshal(results); err == nil {
		if _, writeErr := fmt.Fprintln(os.Stdout, string(payload)); writeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write json: %v\n", writeErr)
		}
	}

	return nil
}

func (c *Client) sendToRoom(ctx context.Context, parsedRoom id.RoomID, message string) (string, error) {
	var encEvt event.EncryptionEventContent
	err := c.Matrix.StateEvent(ctx, parsedRoom, event.StateEncryption, "", &encEvt)
	if err != nil && !errors.Is(err, mautrix.MNotFound) {
		return "", fmt.Errorf("failed to fetch room encryption state: %w", err)
	}

	if err == nil {
		if storeErr := c.Matrix.StateStore.SetEncryptionEvent(ctx, parsedRoom, &encEvt); storeErr != nil {
			return "", fmt.Errorf("failed to store room encryption state: %w", storeErr)
		}

		members, membersErr := c.Matrix.JoinedMembers(ctx, parsedRoom)
		if membersErr != nil {
			return "", fmt.Errorf("failed to fetch room members: %w", membersErr)
		}

		for userID := range members.Joined {
			if setErr := c.Matrix.StateStore.SetMembership(ctx, parsedRoom, userID, event.MembershipJoin); setErr != nil {
				return "", fmt.Errorf("failed to populate state store membership for %s: %w", userID, setErr)
			}
		}
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    message,
	}

	resp, err := c.Matrix.SendMessageEvent(ctx, parsedRoom, event.EventMessage, content)
	if err != nil {
		return "", fmt.Errorf("failed to transmit event: %w", err)
	}

	return string(resp.EventID), nil
}

// Verify initiates an interactive SAS terminal flow.
func (c *Client) Verify(_ context.Context) error {
	_, _ = fmt.Fprintln(os.Stderr, "Waiting for verification requests. Trigger verification from another device...")

	if err := c.Matrix.Sync(); err != nil {
		return fmt.Errorf("verification sync aborted: %w", err)
	}

	return nil
}

// Rooms fetches the list of joined rooms for the authenticated account and outputs it as JSON.
// If verbose is true, it fetches detailed metadata for each room using a progress spinner.
func (c *Client) Rooms(ctx context.Context, verbose bool) error {
	resp, err := c.Matrix.JoinedRooms(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch joined rooms: %w", err)
	}

	if !verbose {
		payload, marshalErr := json.MarshalIndent(resp, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal rooms: %w", marshalErr)
		}
		if _, writeErr := fmt.Fprintln(os.Stdout, string(payload)); writeErr != nil {
			return fmt.Errorf("stdout write error: %w", writeErr)
		}
		return nil
	}

	var completed atomic.Int32
	total := len(resp.JoinedRooms)
	stopSpinner := spinner.Start(ctx, "Fetching room details...", &completed, total)

	var details []RoomDetail
	for _, roomID := range resp.JoinedRooms {
		detail := c.fetchRoomMetadata(ctx, roomID)
		details = append(details, detail)
		completed.Add(1)
	}
	stopSpinner()

	payload, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal detailed rooms: %w", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
		return fmt.Errorf("stdout write error: %w", err)
	}
	return nil
}

func (c *Client) fetchRoomMetadata(ctx context.Context, roomID id.RoomID) RoomDetail {
	detail := RoomDetail{RoomID: string(roomID)}

	var nameEvt event.RoomNameEventContent
	if err := c.Matrix.StateEvent(ctx, roomID, event.StateRoomName, "", &nameEvt); err == nil {
		detail.Name = nameEvt.Name
	}

	var aliasEvt event.CanonicalAliasEventContent
	if err := c.Matrix.StateEvent(ctx, roomID, event.StateCanonicalAlias, "", &aliasEvt); err == nil {
		detail.CanonicalAlias = string(aliasEvt.Alias)
	}

	var topicEvt event.TopicEventContent
	if err := c.Matrix.StateEvent(ctx, roomID, event.StateTopic, "", &topicEvt); err == nil {
		detail.Topic = topicEvt.Topic
	}

	return detail
}

func (c *Client) fetchDetailedRoomMetadata(ctx context.Context, roomID id.RoomID) DetailedRoomInfo {
	info := DetailedRoomInfo{
		RoomDetail: c.fetchRoomMetadata(ctx, roomID),
	}

	var encEvt event.EncryptionEventContent
	if err := c.Matrix.StateEvent(ctx, roomID, event.StateEncryption, "", &encEvt); err == nil {
		info.Encrypted = true
	}

	var createEvt *event.Event
	if evt, err := c.Matrix.FullStateEvent(ctx, roomID, event.StateCreate, ""); err == nil {
		createEvt = evt
		info.Creator = string(evt.Sender)
		if createContent, ok := evt.Content.Parsed.(*event.CreateEventContent); ok {
			info.Version = string(createContent.RoomVersion)
		}
	}

	var plEvt event.PowerLevelsEventContent
	if err := c.Matrix.StateEvent(ctx, roomID, event.StatePowerLevels, "", &plEvt); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to fetch power levels for %s: %v\n", roomID, err)
		plEvt.Users = make(map[id.UserID]int)
	}
	if createEvt != nil {
		plEvt.CreateEvent = createEvt
	}

	if resp, err := c.Matrix.JoinedMembers(ctx, roomID); err == nil {
		info.MemberCount = len(resp.Joined)
		for userID := range resp.Joined {
			level := min(plEvt.GetUserLevel(userID), 100)

			role := "User"
			switch {
			case level >= 100:
				role = "Admin"
			case level >= 50:
				role = "Moderator"
			case level > 0:
				role = "Privileged"
			}

			info.Members = append(info.Members, MemberInfo{
				UserID:     string(userID),
				PowerLevel: level,
				Role:       role,
			})
		}
	}

	return info
}

// RoomInfo fetches and prints the detailed metadata for specific rooms.
func (c *Client) RoomInfo(ctx context.Context, roomsStr string) error {
	roomList := strings.Fields(roomsStr)
	if len(roomList) == 0 {
		return errors.New("no rooms specified")
	}

	var completed atomic.Int32
	stopSpinner := spinner.Start(ctx, "Fetching room information...", &completed, len(roomList))

	results := make([]DetailedRoomInfo, 0, len(roomList))
	for _, r := range roomList {
		detail := c.fetchDetailedRoomMetadata(ctx, id.RoomID(r))
		results = append(results, detail)
		completed.Add(1)
	}
	stopSpinner()

	payload, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal room details: %w", err)
	}

	if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
		return fmt.Errorf("stdout write error: %w", err)
	}
	return nil
}

// Devices fetches the list of active devices for the authenticated account and outputs it as JSON.
func (c *Client) Devices(ctx context.Context) error {
	resp, err := c.Matrix.GetDevicesInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch devices: %w", err)
	}

	payload, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
		return fmt.Errorf("stdout write error: %w", err)
	}
	return nil
}
