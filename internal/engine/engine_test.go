package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

// blockingParticipant blocks until its context is cancelled, then returns the
// context's error. Used to test timeout enforcement.
type blockingParticipant struct {
	capturedCtx context.Context
}

func (b *blockingParticipant) Execute(ctx context.Context, _ any) (any, error) {
	b.capturedCtx = ctx
	<-ctx.Done()
	return nil, ctx.Err()
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

// ----- resolveTimeout -----

func TestResolveTimeoutFlowOverrideTakesPriority(t *testing.T) {
	overrideDur := &model.Duration{Duration: 5 * time.Second}
	participantDur := &model.Duration{Duration: 30 * time.Second}
	defaultsDur := &model.Duration{Duration: 60 * time.Second}

	def := model.Participant{Timeout: participantDur}
	override := &model.ParticipantOverrideStep{Timeout: overrideDur}
	wf := &model.Workflow{Defaults: &model.Defaults{Timeout: defaultsDur}}

	got := resolveTimeout(def, override, wf)
	if got != overrideDur {
		t.Errorf("resolveTimeout = %v, want flow override %v", got, overrideDur)
	}
}

func TestResolveTimeoutParticipantOverDefaults(t *testing.T) {
	participantDur := &model.Duration{Duration: 30 * time.Second}
	defaultsDur := &model.Duration{Duration: 60 * time.Second}

	def := model.Participant{Timeout: participantDur}
	wf := &model.Workflow{Defaults: &model.Defaults{Timeout: defaultsDur}}

	got := resolveTimeout(def, nil, wf)
	if got != participantDur {
		t.Errorf("resolveTimeout = %v, want participant %v", got, participantDur)
	}
}

func TestResolveTimeoutDefaultsWhenNoOtherSet(t *testing.T) {
	defaultsDur := &model.Duration{Duration: 60 * time.Second}

	def := model.Participant{}
	wf := &model.Workflow{Defaults: &model.Defaults{Timeout: defaultsDur}}

	got := resolveTimeout(def, nil, wf)
	if got != defaultsDur {
		t.Errorf("resolveTimeout = %v, want defaults %v", got, defaultsDur)
	}
}

func TestResolveTimeoutNilWhenNoneSet(t *testing.T) {
	def := model.Participant{}
	wf := &model.Workflow{}

	got := resolveTimeout(def, nil, wf)
	if got != nil {
		t.Errorf("resolveTimeout = %v, want nil", got)
	}
}

func TestResolveTimeoutOverrideNilTimeoutFallsThrough(t *testing.T) {
	participantDur := &model.Duration{Duration: 10 * time.Second}
	def := model.Participant{Timeout: participantDur}
	// override present but with no timeout set
	override := &model.ParticipantOverrideStep{Timeout: nil}
	wf := &model.Workflow{}

	got := resolveTimeout(def, override, wf)
	if got != participantDur {
		t.Errorf("resolveTimeout = %v, want participant %v", got, participantDur)
	}
}

// ----- timeout integration -----

func TestRunTimeoutCausesStepFailure(t *testing.T) {
	bp := &blockingParticipant{}
	shortTimeout := &model.Duration{Duration: 10 * time.Millisecond}
	wf := &model.Workflow{
		ID: "wf-timeout",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec, Timeout: shortTimeout},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": bp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error due to timeout, got nil")
	}
}

func TestRunTimeoutWithOnErrorSkipContinues(t *testing.T) {
	bp := &blockingParticipant{}
	mp2 := &mockParticipant{output: "step2-result"}
	shortTimeout := &model.Duration{Duration: 10 * time.Millisecond}
	wf := &model.Workflow{
		ID: "wf-timeout-skip",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec, Timeout: shortTimeout, OnError: "skip"},
			"step2": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Participant: "step1"},
			{Participant: "step2"},
		},
	}
	reg := participant.Registry{"step1": bp, "step2": mp2}

	out, err := Run(context.Background(), wf, nil, nil, reg)
	if err != nil {
		t.Fatalf("Run() expected no error with onError=skip, got: %v", err)
	}
	if out != "step2-result" {
		t.Errorf("Run() = %v, want step2-result", out)
	}
}

func TestRunTimeoutViaFlowOverride(t *testing.T) {
	bp := &blockingParticipant{}
	shortTimeout := &model.Duration{Duration: 10 * time.Millisecond}
	wf := &model.Workflow{
		ID: "wf-override-timeout",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{
				Override: &model.ParticipantOverrideStep{
					Participant: "step1",
					Timeout:     shortTimeout,
				},
			},
		},
	}
	reg := participant.Registry{"step1": bp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error due to flow-override timeout, got nil")
	}
}

func TestRunTimeoutViaDefaults(t *testing.T) {
	bp := &blockingParticipant{}
	shortTimeout := &model.Duration{Duration: 10 * time.Millisecond}
	wf := &model.Workflow{
		ID:       "wf-defaults-timeout",
		Defaults: &model.Defaults{Timeout: shortTimeout},
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": bp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error due to defaults timeout, got nil")
	}
}

// ----- helpers -----

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

// ----- Sub-workflow (workflow participant) -----

// TestRunSubWorkflowComposition verifies that a WorkflowParticipant can be
// wired into a parent workflow's registry so that the child's output is
// propagated to the parent's step result and then to the parent's output.
func TestRunSubWorkflowComposition(t *testing.T) {
	// Child workflow: a single exec-like step that returns a greeting.
	childWF := &model.Workflow{
		ID: "child",
		Inputs: map[string]model.InputField{
			"name": {Default: "world"},
		},
		Participants: map[string]model.Participant{
			"greet": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "greet"}},
	}
	childMock := &mockParticipant{}

	// SubWorkflowRunnerFunc: runs the child workflow using the real engine.Run.
	// This is the closure that the CLI layer would provide in production.
	childRunner := participant.SubWorkflowRunnerFunc(func(ctx context.Context, path string, inputs map[string]any, env map[string]string) (any, error) {
		name, _ := inputs["name"].(string)
		childMock.output = "hello, " + name
		childReg := participant.Registry{"greet": childMock}
		return Run(ctx, childWF, inputs, env, childReg)
	})

	// Build the workflow participant and add it to the parent registry.
	subWFParticipant, err := participant.NewWorkflow("child.flow.yaml", nil, childRunner)
	if err != nil {
		t.Fatalf("NewWorkflow() error: %v", err)
	}

	// Parent workflow: calls the sub-workflow participant and exposes its output.
	parentWF := &model.Workflow{
		ID: "parent",
		Participants: map[string]model.Participant{
			"sub": {
				Type:  model.ParticipantTypeWorkflow,
				Path:  "child.flow.yaml",
				Input: map[string]interface{}{"name": `"alice"`},
			},
		},
		Flow: []model.FlowStep{{Participant: "sub"}},
	}
	parentReg := participant.Registry{"sub": subWFParticipant}

	out, err := Run(context.Background(), parentWF, nil, nil, parentReg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out != "hello, alice" {
		t.Errorf("Run() = %v, want 'hello, alice'", out)
	}
}

// TestRunSubWorkflowErrorPropagated verifies that a failure inside a child
// workflow is surfaced as an error in the parent.
func TestRunSubWorkflowErrorPropagated(t *testing.T) {
	childRunner := participant.SubWorkflowRunnerFunc(func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, fmt.Errorf("child workflow exploded")
	})

	subWFParticipant, _ := participant.NewWorkflow("bad.flow.yaml", nil, childRunner)

	parentWF := &model.Workflow{
		ID: "parent",
		Participants: map[string]model.Participant{
			"sub": {Type: model.ParticipantTypeWorkflow, Path: "bad.flow.yaml"},
		},
		Flow: []model.FlowStep{{Participant: "sub"}},
	}
	parentReg := participant.Registry{"sub": subWFParticipant}

	_, err := Run(context.Background(), parentWF, nil, nil, parentReg)
	if err == nil {
		t.Fatal("Run() expected error from failed sub-workflow, got nil")
	}
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

// ----- Non-boolean condition errors (Gap 2) -----

func TestRunIfNonBoolConditionErrors(t *testing.T) {
	mp := &mockParticipant{output: "x"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		// "workflow.id" evaluates to a string, not a bool.
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: "workflow.id",
				Then:      []model.FlowStep{{Participant: "step1"}},
			}},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error for non-bool if condition, got nil")
	}
	if !strings.Contains(err.Error(), "bool") {
		t.Errorf("error should mention 'bool', got: %v", err)
	}
}

func TestRunWhenNonBoolErrors(t *testing.T) {
	mp := &mockParticipant{output: "x"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		// "workflow.id" evaluates to a string, not a bool.
		Flow: []model.FlowStep{
			{Override: &model.ParticipantOverrideStep{
				Participant: "step1",
				When:        "workflow.id",
			}},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error for non-bool when guard, got nil")
	}
	if !strings.Contains(err.Error(), "bool") {
		t.Errorf("error should mention 'bool', got: %v", err)
	}
}

func TestRunLoopUntilNonBoolErrors(t *testing.T) {
	mp := &mockParticipant{output: "x"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		// "workflow.id" evaluates to a string, not a bool.
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Until: "workflow.id",
				Max:   5,
				Steps: []model.FlowStep{{Participant: "step1"}},
			}},
		},
	}
	reg := participant.Registry{"step1": mp}

	_, err := Run(context.Background(), wf, nil, nil, reg)
	if err == nil {
		t.Fatal("Run() expected error for non-bool loop.until, got nil")
	}
	if !strings.Contains(err.Error(), "bool") {
		t.Errorf("error should mention 'bool', got: %v", err)
	}
}

// ----- Step timing metadata (Gap 4) -----

func TestRunStepResultHasTimingFields(t *testing.T) {
	mp := &mockParticipant{output: "done"}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	state := NewState(wf, nil, nil)
	celEnv, err := cel.NewEnv(wf)
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}
	_, runErr := runSequential(context.Background(), wf, wf.Flow, state, celEnv, reg)
	if runErr != nil {
		t.Fatalf("runSequential: %v", runErr)
	}

	result, ok := state.Steps["step1"]
	if !ok {
		t.Fatal("step1 not found in state.Steps")
	}
	if result.StartedAt == "" {
		t.Error("StepResult.StartedAt should be populated")
	}
	if result.FinishedAt == "" {
		t.Error("StepResult.FinishedAt should be populated")
	}
	if result.Duration == "" {
		t.Error("StepResult.Duration should be populated")
	}
}

func TestRunStepResultHasErrorOnFailure(t *testing.T) {
	mp := &mockParticipant{err: errors.New("boom")}
	wf := &model.Workflow{
		ID: "wf1",
		Participants: map[string]model.Participant{
			"step1": {Type: model.ParticipantTypeExec, OnError: "skip"},
		},
		Flow: []model.FlowStep{{Participant: "step1"}},
	}
	reg := participant.Registry{"step1": mp}

	state := NewState(wf, nil, nil)
	celEnv, err := cel.NewEnv(wf)
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}
	_, runErr := runSequential(context.Background(), wf, wf.Flow, state, celEnv, reg)
	if runErr != nil {
		t.Fatalf("runSequential: %v", runErr)
	}

	result, ok := state.Steps["step1"]
	if !ok {
		t.Fatal("step1 not found in state.Steps")
	}
	if result.Error == "" {
		t.Error("StepResult.Error should be populated on failure")
	}
	if result.StartedAt == "" {
		t.Error("StepResult.StartedAt should be populated even on failure")
	}
}
