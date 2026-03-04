package participant

import "context"

// Participant is the execution interface that all participant types must implement.
type Participant interface {
	Execute(ctx context.Context, input any) (any, error)
}

// Registry maps participant names to their Participant implementations.
// The engine resolves each flow step's participant name against this registry
// before invoking Execute.
type Registry map[string]Participant
