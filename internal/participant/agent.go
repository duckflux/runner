package participant

import (
	"context"
	"errors"

	"github.com/duckflux/runner/internal/model"
)

// AgentExecutor is the interface that v2 agent implementations must satisfy.
// It extends Participant with agent-specific configuration so the engine can
// configure the AI model and available tools before calling Execute.
type AgentExecutor interface {
	Participant
	// SetModel configures the AI model identifier used by the agent.
	SetModel(model string)
	// SetTools configures the list of tool names available to the agent.
	SetTools(tools []string)
}

// AgentParticipant is a stub for the "agent" participant type.
// It satisfies the Participant interface and returns a clear
// "not yet implemented" error on every Execute call.
// A real implementation will be provided in a future release via AgentExecutor.
type AgentParticipant struct {
	def model.Participant
}

// NewAgent constructs an AgentParticipant from a participant definition.
func NewAgent(def model.Participant) *AgentParticipant {
	return &AgentParticipant{def: def}
}

// Execute always returns an error indicating that the agent participant type
// is not yet implemented.
func (a *AgentParticipant) Execute(_ context.Context, _ any) (any, error) {
	return nil, errors.New("agent participant type is not yet implemented")
}
