package participant

import (
	"context"
	"testing"

	"github.com/duckflux/runner/internal/model"
)

// --- MCPParticipant tests ---

func TestMCPExecuteReturnsStructuredResult(t *testing.T) {
	p := NewMCP(model.Participant{Type: model.ParticipantTypeMCP})
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	m := out.(map[string]any)
	if m["status"] != "success" {
		t.Errorf("status = %v, want success", m["status"])
	}
}

func TestMCPExecuteWithInput(t *testing.T) {
	p := NewMCP(model.Participant{
		Type:   model.ParticipantTypeMCP,
		Server: "http://localhost:8080",
		Tool:   "listResources",
	})
	out, err := p.Execute(context.Background(), map[string]any{"filter": "all"})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	m := out.(map[string]any)
	if m["tool"] != "listResources" {
		t.Errorf("tool = %v, want listResources", m["tool"])
	}
}

// Verify MCPParticipant satisfies the Participant interface at compile time.
var _ Participant = (*MCPParticipant)(nil)

// Removed participant types from older specs; no tests.
