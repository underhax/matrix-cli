package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

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

// Rooms fetches the list of joined rooms for the authenticated account and outputs it as JSON.
// If verbose is true, it fetches detailed metadata for each room using a progress spinner.
func (c *Client) Rooms(ctx context.Context, verbose bool) error {
	resp, err := c.Matrix.JoinedRooms(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch joined rooms: %w", err)
	}

	if !verbose {
		payload, marshalErr := jsonMarshalIndent(resp, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal rooms: %w", marshalErr)
		}
		if _, writeErr := fmt.Fprintln(stdout, string(payload)); writeErr != nil {
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

	payload, err := jsonMarshalIndent(details, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal detailed rooms: %w", err)
	}
	if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
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

	payload, err := jsonMarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal room details: %w", err)
	}

	if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
		return fmt.Errorf("stdout write error: %w", err)
	}
	return nil
}

type deviceInfo struct {
	TrustState string `json:"trust_state,omitempty"`
	mautrix.RespDeviceInfo
}

type devicesInfo struct {
	Devices []deviceInfo `json:"devices"`
}

// Devices fetches the list of active devices for the authenticated account and outputs it as JSON.
func (c *Client) Devices(ctx context.Context) error {
	resp, err := c.Matrix.GetDevicesInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch devices: %w", err)
	}

	enriched := devicesInfo{
		Devices: make([]deviceInfo, len(resp.Devices)),
	}
	mach := getOlmMachine(c)

	for i, dev := range resp.Devices {
		enriched.Devices[i] = deviceInfo{
			RespDeviceInfo: dev,
		}
		if mach != nil {
			cryptoDev, errDev := getOrFetchDevice(ctx, mach, c.Matrix.UserID, dev.DeviceID)
			if errDev == nil && cryptoDev != nil {
				trust, errTrust := resolveTrustContext(ctx, mach, cryptoDev)
				if errTrust == nil {
					switch trust {
					case id.TrustStateCrossSignedVerified, id.TrustStateVerified:
						enriched.Devices[i].TrustState = "verified"
					case id.TrustStateCrossSignedTOFU:
						enriched.Devices[i].TrustState = "tofu"
					case id.TrustStateUnset:
						enriched.Devices[i].TrustState = "unverified"
					case id.TrustStateBlacklisted:
						enriched.Devices[i].TrustState = "blacklisted"
					case id.TrustStateCrossSignedUntrusted:
						enriched.Devices[i].TrustState = "untrusted"
					default:
						enriched.Devices[i].TrustState = fmt.Sprintf("unknown (%d)", trust)
					}
				}
			}
		}
	}

	payload, err := jsonMarshalIndent(enriched, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
		return fmt.Errorf("stdout write error: %w", err)
	}
	return nil
}
