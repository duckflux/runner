package participant

import (
	"context"

	"github.com/duckflux/runner/internal/model"
)

// MCPExecutor is the interface that v2 MCP implementations must satisfy.
// It extends Participant with MCP-specific configuration so the engine can
// configure the server endpoint and operation before calling Execute.
type MCPExecutor interface {
	Participant
	// SetServer configures the MCP server endpoint.
	SetServer(server string)
	// SetTool configures the MCP tool/operation to invoke.
	SetTool(tool string)
}

// MCPParticipant provides a deterministic baseline implementation for the
// "mcp" participant type. It returns the configured server/tool and input.
type MCPParticipant struct {
	def    model.Participant
	server string
	tool   string
}

// NewMCP constructs an MCPParticipant from a participant definition.
func NewMCP(def model.Participant) *MCPParticipant {
	return &MCPParticipant{def: def}
}

// Execute returns a structured response with server/tool metadata.
func (m *MCPParticipant) Execute(_ context.Context, input any) (any, error) {
	server := m.server
	if server == "" {
		server = m.def.Server
	}
	tool := m.tool
	if tool == "" {
		tool = m.def.Tool
	}
	return map[string]any{
		"server": server,
		"tool":   tool,
		"output": input,
		"status": "success",
	}, nil
}

// SetServer configures the MCP endpoint for future executions.
func (m *MCPParticipant) SetServer(server string) {
	m.server = server
}

// SetTool configures the tool/operation to be invoked on the MCP server.
func (m *MCPParticipant) SetTool(tool string) {
	m.tool = tool
}
