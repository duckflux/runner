package cel

import (
	"testing"

	"github.com/duckflux/runner/internal/model"
)

// newTestWorkflow builds a minimal Workflow with the given participant names
// and input fields for use in tests.
func newTestWorkflow(participants []string, inputs map[string]model.InputField) *model.Workflow {
	pm := make(map[string]model.Participant, len(participants))
	for _, name := range participants {
		pm[name] = model.Participant{Type: model.ParticipantTypeExec}
	}
	return &model.Workflow{
		ID:           "test-workflow",
		Participants: pm,
		Inputs:       inputs,
	}
}

// ----- NewEnv -----

func TestNewEnvEmpty(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, err := NewEnv(wf)
	if err != nil {
		t.Fatalf("NewEnv() error: %v", err)
	}
	if env == nil {
		t.Fatal("NewEnv() returned nil environment")
	}
}

func TestNewEnvWithParticipants(t *testing.T) {
	wf := newTestWorkflow([]string{"coder", "reviewer"}, nil)
	env, err := NewEnv(wf)
	if err != nil {
		t.Fatalf("NewEnv() error: %v", err)
	}
	if len(env.participants) != 2 {
		t.Errorf("participants len = %d, want 2", len(env.participants))
	}
}

// ----- Compile -----

func TestCompileSimpleBool(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	prog, err := env.Compile("true")
	if err != nil {
		t.Fatalf("Compile(\"true\") error: %v", err)
	}
	if prog == nil {
		t.Fatal("Compile() returned nil program")
	}
}

func TestCompileSyntaxError(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	_, err := env.Compile("!!!invalid{{")
	if err == nil {
		t.Error("expected compilation error for invalid expression, got nil")
	}
}

func TestCompileUndeclaredVariable(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	// "undeclared" is not a known variable — Compile should return an error.
	_, err := env.Compile("undeclared.field == true")
	if err == nil {
		t.Error("expected type-check error for undeclared variable, got nil")
	}
}

func TestCompileDeclaredParticipant(t *testing.T) {
	wf := newTestWorkflow([]string{"reviewer"}, nil)
	env, _ := NewEnv(wf)

	// reviewer is declared, so this should compile without error.
	_, err := env.Compile(`reviewer.output == "approved"`)
	if err != nil {
		t.Fatalf("Compile(reviewer.output) error: %v", err)
	}
}

func TestCompileWorkflowVar(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	_, err := env.Compile(`workflow.id == "my-workflow"`)
	if err != nil {
		t.Fatalf("Compile(workflow.id) error: %v", err)
	}
}

func TestCompileInputVar(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	_, err := env.Compile(`input.repoUrl == "https://github.com/example/repo"`)
	if err != nil {
		t.Fatalf("Compile(input.repoUrl) error: %v", err)
	}
}

func TestCompileEnvVar(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	_, err := env.Compile(`env["TOKEN"] == "secret"`)
	if err != nil {
		t.Fatalf("Compile(env[\"TOKEN\"]) error: %v", err)
	}
}

func TestCompileLoopVar(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	// "loop" is a CEL reserved identifier; the loop context is exposed as "_loop".
	_, err := env.Compile(`_loop.index > 3`)
	if err != nil {
		t.Fatalf("Compile(`_loop.index > 3`) error: %v", err)
	}
}

func TestCompileExecutionVar(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	_, err := env.Compile(`execution.status == "running"`)
	if err != nil {
		t.Fatalf("Compile(execution.status) error: %v", err)
	}
}

// ----- Eval -----

func TestEvalLiteralTrue(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile("true")

	result, err := env.Eval(prog, &State{})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	b, ok := result.(bool)
	if !ok {
		t.Fatalf("Eval() type = %T, want bool", result)
	}
	if !b {
		t.Error("Eval(true) = false, want true")
	}
}

func TestEvalLiteralFalse(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile("false")

	result, err := env.Eval(prog, &State{})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != false {
		t.Errorf("Eval(false) = %v, want false", result)
	}
}

func TestEvalArithmetic(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile("1 + 2")

	result, err := env.Eval(prog, &State{})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != int64(3) {
		t.Errorf("Eval(1+2) = %v (%T), want 3 (int64)", result, result)
	}
}

func TestEvalWorkflowID(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`workflow.id == "my-workflow"`)

	state := &State{
		Workflow: WorkflowMeta{ID: "my-workflow", Name: "My Workflow"},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(workflow.id == 'my-workflow') = %v, want true", result)
	}
}

func TestEvalInputField(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`input.branch == "main"`)

	state := &State{
		Input: map[string]any{"branch": "main"},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(input.branch == 'main') = %v, want true", result)
	}
}

func TestEvalEnvLookup(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`env["TOKEN"] == "secret"`)

	state := &State{
		Env: map[string]string{"TOKEN": "secret"},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(env[TOKEN]) = %v, want true", result)
	}
}

func TestEvalStepOutputField(t *testing.T) {
	wf := newTestWorkflow([]string{"reviewer"}, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`reviewer.output.approved == true`)

	state := &State{
		Steps: map[string]*StepResult{
			"reviewer": {
				Output: map[string]any{"approved": true, "score": int64(9)},
				Status: "success",
			},
		},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(reviewer.output.approved) = %v, want true", result)
	}
}

func TestEvalStepOutputNotYetRun(t *testing.T) {
	// A participant that has not run yet has an empty map binding.
	// Accessing a key that does not exist returns an error; use has() to test existence.
	wf := newTestWorkflow([]string{"reviewer"}, nil)
	env, _ := NewEnv(wf)

	prog, err := env.Compile(`has(reviewer.status)`)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	result, err := env.Eval(prog, &State{Steps: map[string]*StepResult{}})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != false {
		t.Errorf("Eval(has(reviewer.status)) for unrun step = %v, want false", result)
	}
}

func TestEvalLoopContext(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`_loop.index > 2`)

	state := &State{
		Loop: &LoopContext{Index: 3, Iteration: 3, First: false, Last: false},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(_loop.index > 2) = %v, want true (index=3)", result)
	}
}

func TestEvalLoopFirst(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`_loop.first == true`)

	state := &State{
		Loop: &LoopContext{Index: 0, Iteration: 1, First: true, Last: false},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(_loop.first == true) = %v, want true", result)
	}
}

func TestEvalNilLoopContext(t *testing.T) {
	// Outside a loop, _loop context should be an empty map — has() returns false for absent keys.
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	prog, err := env.Compile(`has(_loop.index)`)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	result, err := env.Eval(prog, &State{Loop: nil})
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != false {
		t.Errorf("Eval(has(_loop.index)) outside loop = %v, want false", result)
	}
}

func TestEvalMultipleParticipants(t *testing.T) {
	wf := newTestWorkflow([]string{"coder", "reviewer"}, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`coder.status == "success" && reviewer.output.approved == true`)

	state := &State{
		Steps: map[string]*StepResult{
			"coder": {
				Output: map[string]any{"code": "func main() {}"},
				Status: "success",
			},
			"reviewer": {
				Output: map[string]any{"approved": true},
				Status: "success",
			},
		},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval() = %v, want true", result)
	}
}

func TestEvalStringContains(t *testing.T) {
	wf := newTestWorkflow([]string{"coder"}, nil)
	env, _ := NewEnv(wf)
	prog, _ := env.Compile(`coder.output.code.contains("main")`)

	state := &State{
		Steps: map[string]*StepResult{
			"coder": {
				Output: map[string]any{"code": "func main() {}"},
				Status: "success",
			},
		},
	}
	result, err := env.Eval(prog, state)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != true {
		t.Errorf("Eval(coder.output.code.contains('main')) = %v, want true", result)
	}
}

// ----- Bindings -----

func TestBindingsNilInputsAndEnv(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	vars := env.Bindings(&State{})
	if _, ok := vars["input"]; !ok {
		t.Error("Bindings: missing 'input' key")
	}
	if _, ok := vars["env"]; !ok {
		t.Error("Bindings: missing 'env' key")
	}
	// Should be empty maps, not nil.
	if vars["input"] == nil {
		t.Error("Bindings: 'input' is nil, want empty map")
	}
	if vars["env"] == nil {
		t.Error("Bindings: 'env' is nil, want empty map")
	}
}

func TestBindingsWorkflowMeta(t *testing.T) {
	wf := newTestWorkflow(nil, nil)
	env, _ := NewEnv(wf)

	vars := env.Bindings(&State{
		Workflow: WorkflowMeta{ID: "w1", Name: "W One", Version: "2.0"},
	})

	wfMap, ok := vars["workflow"].(map[string]any)
	if !ok {
		t.Fatalf("vars[workflow] type = %T, want map[string]any", vars["workflow"])
	}
	if wfMap["id"] != "w1" {
		t.Errorf("workflow.id = %v, want w1", wfMap["id"])
	}
	if wfMap["version"] != "2.0" {
		t.Errorf("workflow.version = %v, want 2.0", wfMap["version"])
	}
}

func TestBindingsParticipantDefault(t *testing.T) {
	wf := newTestWorkflow([]string{"coder"}, nil)
	env, _ := NewEnv(wf)

	// No steps run yet: coder should still appear as empty map.
	vars := env.Bindings(&State{})
	if _, ok := vars["coder"]; !ok {
		t.Error("Bindings: declared participant 'coder' missing from bindings")
	}
	m, ok := vars["coder"].(map[string]any)
	if !ok {
		t.Fatalf("vars[coder] type = %T, want map[string]any", vars["coder"])
	}
	if len(m) != 0 {
		t.Errorf("unrun participant should have empty map, got %v", m)
	}
}
