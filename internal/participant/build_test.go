package participant

import (
	"context"
	"testing"

	"github.com/duckflux/runner/internal/model"
)

// ----- BuildRegistry -----

func TestBuildRegistryExec(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"greeter": {Type: model.ParticipantTypeExec, Run: "echo hello"},
		},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["greeter"]; !ok {
		t.Error("BuildRegistry() missing 'greeter' participant")
	}
	if _, ok := reg["greeter"].(*ExecParticipant); !ok {
		t.Errorf("BuildRegistry() greeter is %T, want *ExecParticipant", reg["greeter"])
	}
}

func TestBuildRegistryHTTP(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"api": {Type: model.ParticipantTypeHTTP, URL: "http://example.com"},
		},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["api"].(*HTTPParticipant); !ok {
		t.Errorf("BuildRegistry() api is %T, want *HTTPParticipant", reg["api"])
	}
}

func TestBuildRegistryHuman(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"reviewer": {Type: model.ParticipantTypeHuman, Prompt: "Approve?"},
		},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["reviewer"].(*HumanParticipant); !ok {
		t.Errorf("BuildRegistry() reviewer is %T, want *HumanParticipant", reg["reviewer"])
	}
}

func TestBuildRegistryWorkflowParticipant(t *testing.T) {
	stub := func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, nil
	}
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"sub": {Type: model.ParticipantTypeWorkflow, Path: "sub.flow.yaml"},
		},
	}
	reg, err := BuildRegistry(wf, nil, stub)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["sub"].(*WorkflowParticipant); !ok {
		t.Errorf("BuildRegistry() sub is %T, want *WorkflowParticipant", reg["sub"])
	}
}

func TestBuildRegistryAgent(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"ai": {Type: model.ParticipantTypeAgent},
		},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["ai"].(*AgentParticipant); !ok {
		t.Errorf("BuildRegistry() ai is %T, want *AgentParticipant", reg["ai"])
	}
}

func TestBuildRegistryMCP(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"tool": {Type: model.ParticipantTypeMCP},
		},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["tool"].(*MCPParticipant); !ok {
		t.Errorf("BuildRegistry() tool is %T, want *MCPParticipant", reg["tool"])
	}
}

func TestBuildRegistryHook(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"hook": {Type: model.ParticipantTypeHook},
		},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	if _, ok := reg["hook"].(*HookParticipant); !ok {
		t.Errorf("BuildRegistry() hook is %T, want *HookParticipant", reg["hook"])
	}
}

func TestBuildRegistryUnknownTypeReturnsError(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"mystery": {Type: "unknown"},
		},
	}
	_, err := BuildRegistry(wf, nil, nil)
	if err == nil {
		t.Fatal("BuildRegistry() expected error for unknown type, got nil")
	}
}

func TestBuildRegistryWorkflowParticipantEmptyPath(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"sub": {Type: model.ParticipantTypeWorkflow, Path: ""},
		},
	}
	stub := func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, nil
	}
	_, err := BuildRegistry(wf, nil, stub)
	if err == nil {
		t.Fatal("BuildRegistry() expected error for workflow with empty path, got nil")
	}
}

func TestBuildRegistryEmpty(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{},
	}
	reg, err := BuildRegistry(wf, nil, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() unexpected error: %v", err)
	}
	if len(reg) != 0 {
		t.Errorf("BuildRegistry() registry len = %d, want 0", len(reg))
	}
}

func TestBuildRegistryPropagatesEnvToExec(t *testing.T) {
	wf := &model.Workflow{
		Participants: map[string]model.Participant{
			"step": {Type: model.ParticipantTypeExec, Run: "echo $MY_VAR"},
		},
	}
	extraEnv := map[string]string{"MY_VAR": "hello"}
	reg, err := BuildRegistry(wf, extraEnv, nil)
	if err != nil {
		t.Fatalf("BuildRegistry() error: %v", err)
	}
	ep, ok := reg["step"].(*ExecParticipant)
	if !ok {
		t.Fatalf("BuildRegistry() step is %T, want *ExecParticipant", reg["step"])
	}
	// Verify the extra env was injected by executing the participant.
	out, execErr := ep.Execute(context.Background(), nil)
	if execErr != nil {
		t.Fatalf("Execute() error: %v", execErr)
	}
	if s, _ := out.(string); s != "hello\n" {
		t.Errorf("Execute() output = %q, want %q", s, "hello\n")
	}
}
