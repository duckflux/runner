package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
	"github.com/duckflux/runner/internal/participant"
)

// mockParticipant is a test double that returns a preset output or error.
type mockParticipant struct {
	output any
	err    error
	// capturedInput holds the last input passed to Execute, for assertion.
	capturedInput any
}

func (m *mockParticipant) Execute(_ context.Context, input any) (any, error) {
	m.capturedInput = input
	return m.output, m.err
}

// ----- NewState -----

func TestNewStateDefaultInputs(t *testing.T) {
	wf := &model.Workflow{
		ID: "wf1",
		Inputs: map[string]model.InputField{
			"branch": {Default: "main"},
			"user":   {Default: "alice"},
		},
	}
	state := NewState(wf, nil, nil)

	if state.Input["branch"] != "main" {
		t.Errorf("Input[branch] = %v, want main", state.Input["branch"])
	}
	if state.Input["user"] != "alice" {
		t.Errorf("Input[user] = %v, want alice", state.Input["user"])
	}
}

func TestNewStateCallerInputsOverrideDefaults(t *testing.T) {
	wf := &model.Workflow{
		ID: "wf1",
		Inputs: map[string]model.InputField{
			"branch": {Default: "main"},
		},
	}
	state := NewState(wf, map[string]any{"branch": "dev"}, nil)

	if state.Input["branch"] != "dev" {
		t.Errorf("Input[branch] = %v, want dev", state.Input["branch"])
	}
}

func TestNewStateWorkflowMeta(t *testing.T) {
	wf := &model.Workflow{ID: "wf-test", Name: "Test WF", Version: "1.0"}
	state := NewState(wf, nil, nil)

	if state.Workflow.ID != "wf-test" {
		t.Errorf("Workflow.ID = %q, want wf-test", state.Workflow.ID)
	}
	if state.Workflow.Name != "Test WF" {
		t.Errorf("Workflow.Name = %q, want 'Test WF'", state.Workflow.Name)
	}
	if state.Workflow.Version != "1.0" {
		t.Errorf("Workflow.Version = %q, want 1.0", state.Workflow.Version)
	}
}

func TestNewStateExecutionMetaSet(t *testing.T) {
	wf := &model.Workflow{ID: "x"}
	state := NewState(wf, nil, nil)

	if state.Execution.ID == "" {
		t.Error("Execution.ID should not be empty")
	}
	if state.Execution.StartedAt == "" {
		t.Error("Execution.StartedAt should not be empty")
	}
	if state.Execution.Status != "running" {
		t.Errorf("Execution.Status = %q, want running", state.Execution.Status)
	}
	if state.Execution.Number != 1 {
		t.Errorf("Execution.Number = %d, want 1", state.Execution.Number)
	}
}

func TestNewStateNilEnvBecomesEmpty(t *testing.T) {
	wf := &model.Workflow{ID: "x"}
	state := NewState(wf, nil, nil)
	if state.Env == nil {
		t.Error("Env should be an empty map, not nil")
	}
}

func TestNewStateStepsMapInitialised(t *testing.T) {
	wf := &model.Workflow{ID: "x"}
	state := NewState(wf, nil, nil)
	if state.Steps == nil {
		t.Error("Steps should be an initialised map, not nil")
	}
}

// ----- autoDetectJSON -----

func TestAutoDetectJSONObject(t *testing.T) {
	v := autoDetectJSON(`{"key":"value"}`)
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("autoDetectJSON returned %T, want map[string]any", v)
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, want value", m["key"])
	}
}

func TestAutoDetectJSONArray(t *testing.T) {
	v := autoDetectJSON(`["a","b"]`)
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("autoDetectJSON returned %T, want []any", v)
	}
	if len(s) != 2 {
		t.Errorf("len = %d, want 2", len(s))
	}
}

func TestAutoDetectJSONPlainString(t *testing.T) {
	input := "hello world"
	v := autoDetectJSON(input)
	if v != input {
		t.Errorf("autoDetectJSON(%q) = %v, want unchanged string", input, v)
	}
}

func TestAutoDetectJSONInvalidJSON(t *testing.T) {
	input := "{not valid json}"
	v := autoDetectJSON(input)
	if v != input {
		t.Errorf("autoDetectJSON(%q) = %v, want unchanged", input, v)
	}
}

func TestAutoDetectJSONNonString(t *testing.T) {
	v := autoDetectJSON(42)
	if v != 42 {
		t.Errorf("autoDetectJSON(42) = %v, want 42", v)
	}
}

func TestAutoDetectJSONEmptyString(t *testing.T) {
	v := autoDetectJSON("")
	if v != "" {
		t.Errorf("autoDetectJSON('') = %v, want ''", v)
	}
}

// ----- evalInput -----

func TestEvalInputNil(t *testing.T) {
	celEnv := mustNewEnv(t, nil, nil)
	state := &cel.State{}

	v, err := evalInput(nil, state, celEnv)
	if err != nil {
		t.Fatalf("evalInput(nil) error: %v", err)
	}
	if v != nil {
		t.Errorf("evalInput(nil) = %v, want nil", v)
	}
}

func TestEvalInputCELStringExpression(t *testing.T) {
	celEnv := mustNewEnv(t, nil, nil)
	state := &cel.State{Input: map[string]any{"branch": "main"}}

	v, err := evalInput(`input["branch"]`, state, celEnv)
	if err != nil {
		t.Fatalf("evalInput error: %v", err)
	}
	if v != "main" {
		t.Errorf("evalInput = %v, want main", v)
	}
}

func TestEvalInputCELMapExpression(t *testing.T) {
	celEnv := mustNewEnv(t, nil, nil)
	state := &cel.State{Input: map[string]any{"branch": "dev"}}

	raw := map[string]interface{}{
		"ref": `input["branch"]`,
	}
	v, err := evalInput(raw, state, celEnv)
	if err != nil {
		t.Fatalf("evalInput map error: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("evalInput map returned %T, want map[string]any", v)
	}
	if m["ref"] != "dev" {
		t.Errorf("ref = %v, want dev", m["ref"])
	}
}

func TestEvalInputScalarPassThrough(t *testing.T) {
	celEnv := mustNewEnv(t, nil, nil)
	state := &cel.State{}

	v, err := evalInput(42, state, celEnv)
	if err != nil {
		t.Fatalf("evalInput(42) error: %v", err)
	}
	if v != 42 {
		t.Errorf("evalInput(42) = %v, want 42", v)
	}
}

func TestEvalInputInvalidCELExpression(t *testing.T) {
	celEnv := mustNewEnv(t, nil, nil)
	state := &cel.State{}

	_, err := evalInput("!!!bad{{", state, celEnv)
	if err == nil {
		t.Error("expected error for invalid CEL expression, got nil")
	}
}

// ----- resolveOutput -----

func TestResolveOutputNilUsesLastStep(t *testing.T) {
	celEnv := mustNewEnv(t, []string{"step1"}, nil)
	state := &cel.State{
		Steps: map[string]*cel.StepResult{
			"step1": {Output: "hello", Status: "success"},
		},
	}

	v, err := resolveOutput(nil, state, celEnv, "step1")
	if err != nil {
		t.Fatalf("resolveOutput error: %v", err)
	}
	if v != "hello" {
		t.Errorf("resolveOutput = %v, want hello", v)
	}
}

func TestResolveOutputExpression(t *testing.T) {
	celEnv := mustNewEnv(t, []string{"step1"}, nil)
	state := &cel.State{
		Steps: map[string]*cel.StepResult{
			"step1": {Output: map[string]any{"result": "ok"}, Status: "success"},
		},
	}

	out := &model.WorkflowOutput{Expression: `step1.output.result`}
	v, err := resolveOutput(out, state, celEnv, "step1")
	if err != nil {
		t.Fatalf("resolveOutput error: %v", err)
	}
	if v != "ok" {
		t.Errorf("resolveOutput = %v, want ok", v)
	}
}

func TestResolveOutputMap(t *testing.T) {
	celEnv := mustNewEnv(t, []string{"step1"}, nil)
	state := &cel.State{
		Steps: map[string]*cel.StepResult{
			"step1": {Output: map[string]any{"code": "abc"}, Status: "success"},
		},
	}

	out := &model.WorkflowOutput{
		Map: map[string]string{
			"myCode": "step1.output.code",
		},
	}
	v, err := resolveOutput(out, state, celEnv, "step1")
	if err != nil {
		t.Fatalf("resolveOutput error: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("resolveOutput returned %T, want map[string]any", v)
	}
	if m["myCode"] != "abc" {
		t.Errorf("myCode = %v, want abc", m["myCode"])
	}
}

// ----- Run (integration) -----

func TestRunSingleStep(t *testing.T) {
	mp := &mockParticipant{output: "result-value"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out != "result-value" {
		t.Errorf("Run() = %v, want result-value", out)
	}
}

func TestRunJSONAutoDetection(t *testing.T) {
	mp := &mockParticipant{output: `{"status":"ok","count":3}`}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("Run() returned %T, want map[string]any", out)
	}
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
}

func TestRunInputMapping(t *testing.T) {
	mp := &mockParticipant{output: "done"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {
				Type:  model.ParticipantTypeExec,
				Input: `input["branch"]`,
			},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, map[string]any{"branch": "feat"}, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if mp.capturedInput != "feat" {
		t.Errorf("capturedInput = %v, want feat", mp.capturedInput)
	}
}

func TestRunExplicitOutputExpression(t *testing.T) {
	mp := &mockParticipant{output: map[string]any{"result": "hello"}}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow:   []model.FlowStep{{Participant: "step1"}},
		Output: &model.WorkflowOutput{Expression: "step1.output.result"},
	}
	reg := participant.Registry{"step1": mp}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out != "hello" {
		t.Errorf("Run() = %v, want hello", out)
	}
}

func TestRunExplicitOutputMap(t *testing.T) {
	mp := &mockParticipant{output: map[string]any{"code": "x", "status": "ok"}}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
		Output: &model.WorkflowOutput{Map: map[string]string{
			"finalCode":   "step1.output.code",
			"finalStatus": "step1.output.status",
		}},
	}
	reg := participant.Registry{"step1": mp}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("Run() returned %T, want map[string]any", out)
	}
	if m["finalCode"] != "x" {
		t.Errorf("finalCode = %v, want x", m["finalCode"])
	}
	if m["finalStatus"] != "ok" {
		t.Errorf("finalStatus = %v, want ok", m["finalStatus"])
	}
}

func TestRunStepFailWithOnErrorFail(t *testing.T) {
	mp := &mockParticipant{err: errors.New("step failed")}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec, OnError: "fail"},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
}

func TestRunStepFailWithOnErrorSkip(t *testing.T) {
	mp1 := &mockParticipant{err: errors.New("step1 failed")}
	mp2 := &mockParticipant{output: "step2 output"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec, OnError: "skip"},
			"step2": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Participant: "step1"},
			{Participant: "step2"},
		},
	}
	reg := participant.Registry{"step1": mp1, "step2": mp2}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out != "step2 output" {
		t.Errorf("Run() = %v, want 'step2 output'", out)
	}
}

func TestRunWhenGuardSkipsStep(t *testing.T) {
	mp := &mockParticipant{output: "should not run"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{
				Override: &model.ParticipantOverrideStep{
					Participant: "step1",
					When:        "false",
				},
			},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if mp.capturedInput != nil {
		t.Error("step1 Execute should not have been called when 'when' guard is false")
	}
	// The step should be recorded as skipped.
	// We can't directly check state here, but no error and mp was not called.
}

func TestRunDefaultsOnError(t *testing.T) {
	mp := &mockParticipant{err: errors.New("boom")}
	wf := &model.Workflow{
		ID:       "wf1",
		Defaults: &model.Defaults{OnError: "skip"},
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	// With defaults.onError = "skip", the step failure is swallowed.
	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() expected no error with defaults.onError=skip, got: %v", err)
	}
}

func TestRunMultipleSteps(t *testing.T) {
	calls := []string{}
	makeMP := func(name string) participant.Participant {
		return &mockParticipant{output: name + "-output"}
	}
	_ = calls
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
			"step2": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Participant: "step1"},
			{Participant: "step2"},
		},
	}
	reg := participant.Registry{
		"step1": makeMP("step1"),
		"step2": makeMP("step2"),
	}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// Without an explicit output, the last step's output is returned.
	if out != "step2-output" {
		t.Errorf("Run() = %v, want step2-output", out)
	}
}

func TestRunMissingParticipantInRegistry(t *testing.T) {
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	// Registry is empty — no implementation provided.
	reg := participant.Registry{}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error for missing registry entry, got nil")
	}
}

func TestRunEnvVariablePassedToState(t *testing.T) {
	mp := &mockParticipant{output: "x"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	// We can't observe state.Env directly from the test, but we can verify that
	// Run does not error when env is provided.
	_, err := Run(context.Background(), wf, nil, map[string]string{"TOKEN": "secret"}, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

// ----- helpers -----

// mustNewEnv creates a cel.Environment for a workflow with the given participant
// names and input fields, calling t.Fatal on error.
func mustNewEnv(t *testing.T, participants []string, inputs map[string]model.InputField) *cel.Environment {
	t.Helper()
	pm := make(map[string]model.Participant, len(participants))
	for _, name := range participants {
		pm[name] = model.Participant{Type: model.ParticipantTypeExec}
	}
	env, err := cel.NewEnv(&model.Workflow{
		ID:           "test",
		Participants: pm,
		Inputs:       inputs,
	})
	if err != nil {
		t.Fatalf("cel.NewEnv error: %v", err)
	}
	return env
}
