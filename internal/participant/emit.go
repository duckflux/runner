package participant

import (
	"context"
	"fmt"
	"time"

	"github.com/duckflux/runner/internal/eventhub"
	"github.com/duckflux/runner/internal/model"
)

// EmitParticipant publishes a named event to the event hub.
//
// When Ack is true, it waits for the publish to be confirmed (using the
// participant timeout from the step context). If the ack times out:
//   - OnTimeout == "skip" → returns success with ack:false in the result
//   - OnTimeout == "" or "fail" (default) → returns an error
type EmitParticipant struct {
	def model.Participant
	hub *eventhub.Hub
}

// NewEmit constructs an EmitParticipant from a participant definition and a
// Hub reference. hub may be nil in tests that do not require real pub/sub.
func NewEmit(def model.Participant, hub *eventhub.Hub) *EmitParticipant {
	return &EmitParticipant{def: def, hub: hub}
}

// WithDefinition returns a copy configured from def, preserving the Hub reference.
func (e *EmitParticipant) WithDefinition(def model.Participant) *EmitParticipant {
	return &EmitParticipant{def: def, hub: e.hub}
}

// Execute publishes the configured event via the event hub and returns a
// structured result map: {event, payload, ack}.
//
// The ctx passed by the engine already carries the participant's timeout
// deadline (from withStepTimeout). For ack mode, Execute uses that deadline
// directly through PublishAndWaitAck.
func (e *EmitParticipant) Execute(ctx context.Context, _ any) (any, error) {
	evt := e.def.Event
	payload := e.def.Payload

	if e.hub != nil {
		if e.def.Ack {
			timeout := 30 * time.Second
			if e.def.Timeout != nil {
				timeout = e.def.Timeout.Duration
			}
			if err := e.hub.PublishAndWaitAck(ctx, evt, payload, timeout); err != nil {
				if e.def.OnTimeout == "skip" {
					return map[string]any{"event": evt, "payload": payload, "ack": false}, nil
				}
				return nil, fmt.Errorf("emit: ack failed for event %q: %w", evt, err)
			}
		} else {
			if err := e.hub.Publish(ctx, evt, payload); err != nil {
				return nil, fmt.Errorf("emit: publish failed for event %q: %w", evt, err)
			}
		}
	}

	return map[string]any{"event": evt, "payload": payload, "ack": e.def.Ack}, nil
}
