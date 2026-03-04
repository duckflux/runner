package participant

import (
	"context"
	"errors"

	"github.com/duckflux/runner/internal/model"
)

// HookExecutor is the interface that v2 hook implementations must satisfy.
// Hooks depend on external signal delivery; this interface extends Participant
// with a Notify method so that signals can be delivered at runtime.
type HookExecutor interface {
	Participant
	// Notify delivers a named signal with an optional payload to the hook.
	Notify(ctx context.Context, signal string, payload any) error
}

// HookParticipant is a stub for the "hook" participant type.
// It satisfies the Participant interface and returns a clear
// "not yet implemented" error on every Execute call.
// A real implementation will be provided in a future release via HookExecutor.
type HookParticipant struct {
	def model.Participant
}

// NewHook constructs a HookParticipant from a participant definition.
func NewHook(def model.Participant) *HookParticipant {
	return &HookParticipant{def: def}
}

// Execute always returns an error indicating that the hook participant type
// is not yet implemented.
func (h *HookParticipant) Execute(_ context.Context, _ any) (any, error) {
	return nil, errors.New("hook participant type is not yet implemented")
}
