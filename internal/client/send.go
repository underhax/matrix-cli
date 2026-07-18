package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

// Send fetches the room membership topology, populates the state store for key distribution,
// and dispatches the message through the CryptoHelper auto-encryption pipeline to multiple rooms.
func (c *Client) Send(ctx context.Context, roomsStr, message string, isHTML, isMarkdown bool) error {
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

		eventID, err := c.sendToRoom(ctx, parsedRoom, message, isHTML, isMarkdown)
		if err != nil {
			res[jsonKeyStatus] = "error"
			res["error"] = err.Error()
		} else {
			res[jsonKeyStatus] = statusSuccess
			res["event_id"] = eventID
		}
		results = append(results, res)
	}

	if payload, err := jsonMarshal(results); err == nil {
		if _, writeErr := fmt.Fprintln(stdout, string(payload)); writeErr != nil {
			if _, printErr := fmt.Fprintf(stderr, "failed to write json: %v\n", writeErr); printErr != nil {
				return fmt.Errorf("failed to output result: %w", writeErr)
			}
			return fmt.Errorf("failed to output result: %w", writeErr)
		}
	}

	return nil
}

func (c *Client) sendToRoom(ctx context.Context, parsedRoom id.RoomID, message string, isHTML, isMarkdown bool) (string, error) {
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

	var content *event.MessageEventContent
	if isMarkdown {
		rendered := format.RenderMarkdown(message, true, isHTML)
		content = &rendered
	} else {
		content = &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    message,
		}
		if isHTML {
			content.Format = event.FormatHTML
			content.FormattedBody = message
		}
	}

	resp, err := c.Matrix.SendMessageEvent(ctx, parsedRoom, event.EventMessage, content)
	if err != nil {
		return "", fmt.Errorf("failed to transmit event: %w", err)
	}

	return string(resp.EventID), nil
}
