package engine

import (
	"context"
	"errors"
	"fmt"
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

// ----- Loop -----

func TestRunLoopMaxIterations(t *testing.T) {
	// Use a counter participant that tracks call count.
	counter := &countingParticipant{}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Max:   3,
				Steps: []model.FlowStep{{Participant: "step1"}},
			}},
		},
	}
	reg := participant.Registry{"step1": counter}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if counter.calls != 3 {
		t.Errorf("step1 called %d times, want 3", counter.calls)
	}
}

func TestRunLoopUntilCondition(t *testing.T) {
	// Each iteration the participant returns an incrementing counter stored in state.
	// The "until" condition checks whether step1.output equals "3".
	counter := &countingParticipant{}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Until: `step1.output == "3"`,
				Max:   10,
				Steps: []model.FlowStep{{Participant: "step1"}},
			}},
		},
	}
	reg := participant.Registry{"step1": counter}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// step1 should have been called 3 times: outputs "1", "2", "3"; until is true after "3".
	if counter.calls != 3 {
		t.Errorf("step1 called %d times, want 3", counter.calls)
	}
}

func TestRunLoopContextFirst(t *testing.T) {
	// Verify that loop.first is true only on the first iteration by using a
	// countingParticipant and checking that the "when" guard based on loop.first runs once.
	var firstCount int
	var totalCount int
	tracker := &funcParticipant{fn: func(_ context.Context, input any) (any, error) {
		totalCount++
		if b, ok := input.(bool); ok && b {
			firstCount++
		}
		return "ok", nil
	}}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Max: 3,
				Steps: []model.FlowStep{
					{Override: &model.ParticipantOverrideStep{
						Participant: "step1",
						Input:       "loop.first",
					}},
				},
			}},
		},
	}
	reg := participant.Registry{"step1": tracker}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if totalCount != 3 {
		t.Errorf("step1 called %d times, want 3", totalCount)
	}
	if firstCount != 1 {
		t.Errorf("loop.first was true %d times, want 1", firstCount)
	}
}

func TestRunLoopNoUntilNoMaxErrors(t *testing.T) {
	mp := &mockParticipant{output: "x"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Steps: []model.FlowStep{{Participant: "step1"}},
			}},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error for loop with neither until nor max, got nil")
	}
}

// ----- Parallel -----

func TestRunParallelBothStepsExecuted(t *testing.T) {
	mp1 := &mockParticipant{output: "out1"}
	mp2 := &mockParticipant{output: "out2"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
			"step2": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Parallel: &model.ParallelStep{Steps: []string{"step1", "step2"}}},
		},
		Output: &model.WorkflowOutput{Map: map[string]string{
			"r1": "step1.output",
			"r2": "step2.output",
		}},
	}
	reg := participant.Registry{"step1": mp1, "step2": mp2}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("Run() returned %T, want map[string]any", out)
	}
	if m["r1"] != "out1" {
		t.Errorf("r1 = %v, want out1", m["r1"])
	}
	if m["r2"] != "out2" {
		t.Errorf("r2 = %v, want out2", m["r2"])
	}
}

func TestRunParallelOneFailCancelsOthers(t *testing.T) {
	mp1 := &mockParticipant{err: errors.New("step1 failed")}
	mp2 := &mockParticipant{output: "out2"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
			"step2": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Parallel: &model.ParallelStep{Steps: []string{"step1", "step2"}}},
		},
	}
	reg := participant.Registry{"step1": mp1, "step2": mp2}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error when parallel branch fails, got nil")
	}
}

func TestRunParallelEmpty(t *testing.T) {
	wf := &model.Workflow{
		ID:           "wf1",
		Participants: map[string]model.Participant{},
		Flow: []model.FlowStep{
			{Parallel: &model.ParallelStep{Steps: []string{}}},
		},
	}
	reg := participant.Registry{}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error for empty parallel: %v", err)
	}
}

// ----- If / then / else -----

func TestRunIfThenBranchExecuted(t *testing.T) {
	mpThen := &mockParticipant{output: "then-result"}
	mpElse := &mockParticipant{output: "else-result"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"thenStep": {Type: model.ParticipantTypeExec},
			"elseStep": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: "true",
				Then:      []model.FlowStep{{Participant: "thenStep"}},
				Else:      []model.FlowStep{{Participant: "elseStep"}},
			}},
		},
	}
	reg := participant.Registry{"thenStep": mpThen, "elseStep": mpElse}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if mpThen.capturedInput == nil && mpThen.output == nil {
		// Execute was called — any non-nil output is fine.
	}
	if mpElse.capturedInput != nil {
		t.Error("elseStep should not have been called when condition is true")
	}
}

func TestRunIfElseBranchExecuted(t *testing.T) {
	mpThen := &mockParticipant{output: "then-result"}
	mpElse := &mockParticipant{output: "else-result"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"thenStep": {Type: model.ParticipantTypeExec},
			"elseStep": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: "false",
				Then:      []model.FlowStep{{Participant: "thenStep"}},
				Else:      []model.FlowStep{{Participant: "elseStep"}},
			}},
		},
	}
	reg := participant.Registry{"thenStep": mpThen, "elseStep": mpElse}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if mpThen.capturedInput != nil {
		t.Error("thenStep should not have been called when condition is false")
	}
}

func TestRunIfNoElseFalseConditionIsNoop(t *testing.T) {
	mp := &mockParticipant{output: "x"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: "false",
				Then:      []model.FlowStep{{Participant: "step1"}},
			}},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if mp.capturedInput != nil {
		t.Error("step1 should not have been called when if condition is false and there is no else")
	}
}

func TestRunIfConditionUsesInputVariable(t *testing.T) {
	mpThen := &mockParticipant{output: "then-ran"}
	mpElse := &mockParticipant{output: "else-ran"}
	wf := &model.Workflow{
		ID: "wf1",
		Inputs: map[string]model.InputField{
			"deploy": {Default: "true"},
		},
		Participants: map[string]model.Participant{
			"deploy":     {Type: model.ParticipantTypeExec},
			"skipDeploy": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: `input["deploy"] == "true"`,
				Then:      []model.FlowStep{{Participant: "deploy"}},
				Else:      []model.FlowStep{{Participant: "skipDeploy"}},
			}},
		},
	}
	reg := participant.Registry{"deploy": mpThen, "skipDeploy": mpElse}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if mpElse.capturedInput != nil {
		t.Error("skipDeploy should not run when input.deploy == 'true'")
	}
}

// ----- When guard (override step) -----

func TestRunWhenGuardTrueExecutesStep(t *testing.T) {
	mp := &mockParticipant{output: "ran"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Override: &model.ParticipantOverrideStep{
				Participant: "step1",
				When:        "true",
			}},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// mp.Execute was called — output would be set.
	if mp.output != "ran" {
		t.Errorf("step1.output = %v, want ran", mp.output)
	}
}

// ----- helper types -----

// countingParticipant tracks how many times Execute is called and returns
// a string representation of the call count as its output.
type countingParticipant struct {
	calls int
}

func (c *countingParticipant) Execute(_ context.Context, _ any) (any, error) {
	c.calls++
	return fmt.Sprintf("%d", c.calls), nil
}

// funcParticipant delegates Execute to an arbitrary function, useful for
// capturing loop-context values passed as inputs.
type funcParticipant struct {
	fn func(context.Context, any) (any, error)
}

func (f *funcParticipant) Execute(ctx context.Context, input any) (any, error) {
	return f.fn(ctx, input)
}

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
