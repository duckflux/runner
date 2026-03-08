package participant

import (
	"context"
	"fmt"

	"github.com/duckflux/runner/internal/model"
)

// EmitParticipant is a minimal implementation of the "emit" participant
// type. In v1 this is a stub: it logs the event and returns success.
type EmitParticipant struct {
	def model.Participant
}

// NewEmit constructs an EmitParticipant from a participant definition.
func NewEmit(def model.Participant) *EmitParticipant {
	return &EmitParticipant{def: def}
}

// WithDefinition returns a copy configured from def.
func (e *EmitParticipant) WithDefinition(def model.Participant) *EmitParticipant {
	return &EmitParticipant{def: def}
}

// Execute publishes the configured event. CEL evaluation of the payload is
// responsibility of the parser/engine; here we treat payload as a literal
// value and return a simple acknowledgement. If Ack is true we simulate an
// acknowledgement by returning success immediately (stubbed).
func (e *EmitParticipant) Execute(ctx context.Context, input any) (any, error) {
	// Use the definition fields directly; payload may be a literal or a
	// pre-evaluated structure provided by higher layers in future.
	evt := e.def.Event
	payload := e.def.Payload

	// Log to stdout for now (stubbing event hub integration).
	// Keep the behavior deterministic and test-friendly.
	msg := fmt.Sprintf("emit: event=%s ack=%v payload=%v", evt, e.def.Ack, payload)
	fmt.Println(msg)

	// Return a simple structured result so callers can inspect what was sent.
	res := map[string]any{
		"event":   evt,
		"payload": payload,
		"ack":     e.def.Ack,
	}
	return res, nil
}
