package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
