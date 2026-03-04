package participant

import (
	"context"
	"strings"
	"testing"

	"github.com/duckflux/runner/internal/model"
)

// --- AgentParticipant tests ---

func TestAgentExecuteReturnsNotImplementedError(t *testing.T) {
	p := NewAgent(model.Participant{Type: model.ParticipantTypeAgent})
	out, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if out != nil {
		t.Errorf("Execute() output = %v, want nil", out)
	}
	if !strings.Contains(err.Error(), "agent") {
		t.Errorf("error = %q, expected it to mention 'agent'", err.Error())
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error = %q, expected it to contain 'not yet implemented'", err.Error())
	}
}

func TestAgentExecuteWithInputReturnsError(t *testing.T) {
	p := NewAgent(model.Participant{
		Type:  model.ParticipantTypeAgent,
		Model: "gpt-4",
		Tools: []string{"search", "calculator"},
	})
	out, err := p.Execute(context.Background(), map[string]any{"prompt": "hello"})
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if out != nil {
		t.Errorf("Execute() output = %v, want nil", out)
	}
}

// Verify AgentParticipant satisfies the Participant interface at compile time.
var _ Participant = (*AgentParticipant)(nil)

// --- MCPParticipant tests ---

func TestMCPExecuteReturnsNotImplementedError(t *testing.T) {
	p := NewMCP(model.Participant{Type: model.ParticipantTypeMCP})
	out, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if out != nil {
		t.Errorf("Execute() output = %v, want nil", out)
	}
	if !strings.Contains(err.Error(), "mcp") {
		t.Errorf("error = %q, expected it to mention 'mcp'", err.Error())
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error = %q, expected it to contain 'not yet implemented'", err.Error())
	}
}

func TestMCPExecuteWithInputReturnsError(t *testing.T) {
	p := NewMCP(model.Participant{
		Type:      model.ParticipantTypeMCP,
		Server:    "http://localhost:8080",
		Operation: "listResources",
	})
	out, err := p.Execute(context.Background(), map[string]any{"filter": "all"})
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if out != nil {
		t.Errorf("Execute() output = %v, want nil", out)
	}
}

// Verify MCPParticipant satisfies the Participant interface at compile time.
var _ Participant = (*MCPParticipant)(nil)

// --- HookParticipant tests ---

func TestHookExecuteReturnsNotImplementedError(t *testing.T) {
	p := NewHook(model.Participant{Type: model.ParticipantTypeHook})
	out, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if out != nil {
		t.Errorf("Execute() output = %v, want nil", out)
	}
	if !strings.Contains(err.Error(), "hook") {
		t.Errorf("error = %q, expected it to mention 'hook'", err.Error())
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error = %q, expected it to contain 'not yet implemented'", err.Error())
	}
}

func TestHookExecuteWithInputReturnsError(t *testing.T) {
	p := NewHook(model.Participant{Type: model.ParticipantTypeHook})
	out, err := p.Execute(context.Background(), "some-signal-payload")
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if out != nil {
		t.Errorf("Execute() output = %v, want nil", out)
	}
}

// Verify HookParticipant satisfies the Participant interface at compile time.
var _ Participant = (*HookParticipant)(nil)
