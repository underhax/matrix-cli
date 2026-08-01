package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/underhax/matrix-cli/internal/ui/spinner"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
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

// Rooms fetches the list of joined rooms for the authenticated account.
// It supports both human-readable and JSON output formats.
// If verbose is true, it fetches detailed metadata for each room.
func (c *Client) Rooms(ctx context.Context, verbose bool) error {
	resp, err := c.Matrix.JoinedRooms(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch joined rooms: %w", err)
	}

	if !verbose {
		return c.printBasicRooms(resp)
	}

	var completed atomic.Int32
	total := len(resp.JoinedRooms)
	stopSpinner := func() {}
	if !JSONMode {
		stopSpinner = spinner.Start(ctx, "Fetching room details...", &completed, total)
	}

	var details []RoomDetail
	for _, roomID := range resp.JoinedRooms {
		detail := c.fetchRoomMetadata(ctx, roomID)
		details = append(details, detail)
		completed.Add(1)
	}
	stopSpinner()

	return c.printDetailedRooms(details)
}

func (c *Client) printBasicRooms(resp *mautrix.RespJoinedRooms) error {
	if JSONMode {
		payload, marshalErr := jsonMarshalIndent(resp, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal rooms: %w", marshalErr)
		}
		if _, writeErr := fmt.Fprintln(stdout, string(payload)); writeErr != nil {
			return fmt.Errorf("stdout write error: %w", writeErr)
		}
		return nil
	}
	for _, roomID := range resp.JoinedRooms {
		if _, writeErr := fmt.Fprintf(stdout, "- %s\n", roomID); writeErr != nil {
			return fmt.Errorf("stdout write error: %w", writeErr)
		}
	}
	return nil
}

func (c *Client) printDetailedRooms(details []RoomDetail) error {
	if JSONMode {
		payload, err := jsonMarshalIndent(details, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal detailed rooms: %w", err)
		}
		if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
			return fmt.Errorf("stdout write error: %w", err)
		}
		return nil
	}
	for i := range details {
		detail := &details[i]
		var parts []string
		if detail.Name != "" {
			parts = append(parts, "Name: "+detail.Name)
		}
		if detail.CanonicalAlias != "" {
			parts = append(parts, "Alias: "+detail.CanonicalAlias)
		}
		infoStr := ""
		if len(parts) > 0 {
			infoStr = " (" + strings.Join(parts, ", ") + ")"
		}
		if _, err := fmt.Fprintf(stdout, "- %s%s\n", detail.RoomID, infoStr); err != nil {
			return fmt.Errorf("stdout write error: %w", err)
		}
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

// RoomInfo fetches and prints detailed metadata for specific rooms.
// It supports both human-readable and JSON output formats.
func (c *Client) RoomInfo(ctx context.Context, roomsStr string) error {
	roomList := strings.Fields(roomsStr)
	if len(roomList) == 0 {
		return errors.New("no rooms specified")
	}

	var completed atomic.Int32
	stopSpinner := func() {}
	if !JSONMode {
		stopSpinner = spinner.Start(ctx, "Fetching room information...", &completed, len(roomList))
	}

	results := make([]DetailedRoomInfo, 0, len(roomList))
	for _, r := range roomList {
		detail := c.fetchDetailedRoomMetadata(ctx, id.RoomID(r))
		results = append(results, detail)
		completed.Add(1)
	}
	stopSpinner()

	return c.printRoomInfoResults(results)
}

func (c *Client) printRoomInfoResults(results []DetailedRoomInfo) error {
	if JSONMode {
		payload, err := jsonMarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal room details: %w", err)
		}

		if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
			return fmt.Errorf("stdout write error: %w", err)
		}
		return nil
	}

	for i := range results {
		if err := c.printSingleRoomInfo(&results[i]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) printSingleRoomInfo(res *DetailedRoomInfo) error {
	var writeErr error
	printf := func(format string, args ...any) {
		if writeErr != nil {
			return
		}
		if _, err := fmt.Fprintf(stdout, format, args...); err != nil {
			writeErr = fmt.Errorf("stdout write error: %w", err)
		}
	}

	printf("\nRoom: %s\n", res.RoomID)
	if res.Name != "" {
		printf("  Name: %s\n", res.Name)
	}
	if res.CanonicalAlias != "" {
		printf("  Alias: %s\n", res.CanonicalAlias)
	}
	if res.Topic != "" {
		printf("  Topic: %s\n", res.Topic)
	}
	printf("  Creator: %s\n", res.Creator)
	printf("  Version: %s\n", res.Version)
	printf("  Encrypted: %t\n", res.Encrypted)
	printf("  Members (%d):\n", res.MemberCount)
	for _, m := range res.Members {
		printf("    - %s (%s, Power Level: %d)\n", m.UserID, m.Role, m.PowerLevel)
	}
	return writeErr
}

type deviceInfo struct {
	TrustState string `json:"trust_state,omitempty"`
	mautrix.RespDeviceInfo
}

type devicesInfo struct {
	Devices []deviceInfo `json:"devices"`
}

func trustStateToString(trust id.TrustState) string {
	switch trust {
	case id.TrustStateCrossSignedVerified, id.TrustStateVerified:
		return "verified"
	case id.TrustStateCrossSignedTOFU:
		return "tofu"
	case id.TrustStateUnset:
		return "unverified"
	case id.TrustStateBlacklisted:
		return "blacklisted"
	case id.TrustStateCrossSignedUntrusted:
		return "untrusted"
	default:
		return fmt.Sprintf("unknown (%d)", trust)
	}
}

// Devices fetches the list of active devices for the authenticated account.
// It supports both human-readable and JSON output formats.
func (c *Client) Devices(ctx context.Context) error {
	resp, err := c.Matrix.GetDevicesInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch devices: %w", err)
	}

	enriched := devicesInfo{
		Devices: make([]deviceInfo, len(resp.Devices)),
	}
	mach := getOlmMachine(c)

	if mach != nil && c.Matrix != nil {
		c.refreshCrossSigningKeys(ctx, c.Matrix.UserID)
	}

	for i, dev := range resp.Devices {
		enriched.Devices[i] = deviceInfo{
			RespDeviceInfo: dev,
		}
		if mach != nil {
			cryptoDev, errDev := getOrFetchDevice(ctx, mach, c.Matrix.UserID, dev.DeviceID)
			if errDev == nil && cryptoDev != nil {
				trust, errTrust := resolveTrustContext(ctx, mach, cryptoDev)
				if errTrust == nil {
					enriched.Devices[i].TrustState = trustStateToString(trust)
				}
			}
		}
	}

	return c.printDevices(enriched)
}

func (c *Client) printDevices(enriched devicesInfo) error {
	if JSONMode {
		payload, err := jsonMarshalIndent(enriched, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal devices: %w", err)
		}

		if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
			return fmt.Errorf("stdout write error: %w", err)
		}
		return nil
	}

	var writeErr error
	printf := func(format string, args ...any) {
		if writeErr != nil {
			return
		}
		if _, err := fmt.Fprintf(stdout, format, args...); err != nil {
			writeErr = fmt.Errorf("stdout write error: %w", err)
		}
	}

	printf("\nDevices (%d):\n", len(enriched.Devices))
	for i := range enriched.Devices {
		d := &enriched.Devices[i]
		trustStr := d.TrustState
		if trustStr == "" {
			trustStr = "unknown"
		}
		if d.DisplayName != "" {
			printf("  - %s (%s)\n", d.DeviceID, d.DisplayName)
		} else {
			printf("  - %s\n", d.DeviceID)
		}
		printf("    Trust: %s\n", trustStr)
		if d.LastSeenIP != "" {
			printf("    Last Seen IP: %s\n", d.LastSeenIP)
		}
		if d.LastSeenTS > 0 {
			ts := time.UnixMilli(d.LastSeenTS).Format("2006-01-02 15:04:05")
			printf("    Last Seen: %s\n", ts)
		}
	}
	return writeErr
}
