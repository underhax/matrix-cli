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

// Send fetches the room membership topology, populates the state store,
// and dispatches the message with auto-encryption to multiple rooms.
// It supports sending plain text, HTML, and Markdown messages.
// It supports both human-readable and JSON output formats.
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

	if JSONMode {
		payload, err := jsonMarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		if _, writeErr := fmt.Fprintln(stdout, string(payload)); writeErr != nil {
			return fmt.Errorf("stdout write error: %w", writeErr)
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

	for _, res := range results {
		roomID := res["room_id"]
		if res[jsonKeyStatus] == statusSuccess {
			printf("Successfully sent message to %s (Event ID: %s)\n", roomID, res["event_id"])
		} else {
			printf("Failed to send message to %s: %s\n", roomID, res["error"])
		}
	}
	return writeErr
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
