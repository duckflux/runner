package participant

import (
	"context"
	"errors"

	"github.com/duckflux/runner/internal/model"
)

// MCPExecutor is the interface that v2 MCP implementations must satisfy.
// It extends Participant with MCP-specific configuration so the engine can
// configure the server endpoint and operation before calling Execute.
type MCPExecutor interface {
	Participant
	// SetServer configures the MCP server endpoint.
	SetServer(server string)
	// SetOperation configures the MCP operation to invoke.
	SetOperation(operation string)
}

// MCPParticipant is a stub for the "mcp" participant type.
// It satisfies the Participant interface and returns a clear
// "not yet implemented" error on every Execute call.
// A real implementation will be provided in a future release via MCPExecutor.
type MCPParticipant struct {
	def model.Participant
}

// NewMCP constructs an MCPParticipant from a participant definition.
func NewMCP(def model.Participant) *MCPParticipant {
	return &MCPParticipant{def: def}
}

// Execute always returns an error indicating that the mcp participant type
// is not yet implemented.
func (m *MCPParticipant) Execute(_ context.Context, _ any) (any, error) {
	return nil, errors.New("mcp participant type is not yet implemented")
}
