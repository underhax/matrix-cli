package client

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

var (
	brRe       = regexp.MustCompile(`(?i)[ \t]*\r?\n?[ \t]*<br\s*/?>[ \t]*\r?\n?[ \t]*`)
	htmlTagsRe = regexp.MustCompile(`<[^>]+>`)
)

func cleanMessageBody(body string) string {
	s := brRe.ReplaceAllString(body, "\n")
	s = htmlTagsRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// Listen starts an infinite sync loop, decrypting and outputting E2EE events.
// It supports both human-readable and JSON output formats.
// If roomsStr is provided, it filters incoming events to the specified room IDs.
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

		if JSONMode {
			c.handleJSONEvent(evt)
		} else {
			c.handleHumanEvent(evt)
		}
	})

	if JSONMode {
		if _, err := fmt.Fprintln(stdout, `{"status": "listening"}`); err != nil {
			return fmt.Errorf("sync loop terminated: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(stderr, "Listening for incoming messages..."); err != nil {
			return fmt.Errorf("sync loop terminated: %w", err)
		}
	}

	if err := c.Matrix.Sync(); err != nil {
		return fmt.Errorf("sync loop terminated: %w", err)
	}

	return nil
}

func (c *Client) handleJSONEvent(evt *event.Event) {
	payload, err := jsonMarshal(evt)
	if err != nil {
		c.Log.Error().Err(err).Msgf("failed to marshal event %s", evt.ID)
		return
	}
	if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
		c.Log.Error().Err(err).Msg("stdout write error")
	}
}

func (c *Client) handleHumanEvent(evt *event.Event) {
	tm := time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05")
	body := "<unparsed message>"
	if content, ok := evt.Content.Parsed.(*event.MessageEventContent); ok {
		body = cleanMessageBody(content.Body)
	}
	if _, err := fmt.Fprintf(stdout, "[%s] [%s] %s:\n%s\n", tm, evt.RoomID, evt.Sender, body); err != nil {
		c.Log.Error().Err(err).Msg("stdout write error")
	}
}
