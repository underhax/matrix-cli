package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

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

		payload, err := jsonMarshal(evt)
		if err != nil {
			if _, writeErr := fmt.Fprintf(stderr, "failed to marshal event %s: %v\n", evt.ID, err); writeErr != nil {
				return
			}
			return
		}
		if _, err := fmt.Fprintln(stdout, string(payload)); err != nil {
			if _, writeErr := fmt.Fprintf(stderr, "stdout write error: %v\n", err); writeErr != nil {
				return
			}
		}
	})

	if _, err := fmt.Fprintln(stderr, "starting infinite sync loop..."); err != nil {
		return fmt.Errorf("failed to write to stderr: %w", err)
	}

	if err := c.Matrix.Sync(); err != nil {
		return fmt.Errorf("sync loop terminated: %w", err)
	}

	return nil
}
