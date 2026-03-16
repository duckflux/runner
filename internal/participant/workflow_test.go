package participant

import (
	"context"
	"errors"
	"testing"
)

// ----- NewWorkflow -----

func TestNewWorkflowEmptyPathReturnsError(t *testing.T) {
	_, err := NewWorkflow("", nil, func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("NewWorkflow() expected error for empty path, got nil")
	}
}

func TestNewWorkflowNilRunnerReturnsError(t *testing.T) {
	_, err := NewWorkflow("sub.flow.yaml", nil, nil)
	if err == nil {
		t.Fatal("NewWorkflow() expected error for nil runnerFn, got nil")
	}
}

func TestNewWorkflowValidArgsSucceeds(t *testing.T) {
	_, err := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewWorkflow() unexpected error: %v", err)
	}
}

// ----- Execute -----

func TestWorkflowExecutePassesPathToRunner(t *testing.T) {
	const wantPath = "child.flow.yaml"
	var gotPath string

	p, _ := NewWorkflow(wantPath, nil, func(_ context.Context, path string, _ map[string]any, _ map[string]string) (any, error) {
		gotPath = path
		return "ok", nil
	})

	if _, err := p.Execute(context.Background(), nil); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotPath != wantPath {
		t.Errorf("runner received path %q, want %q", gotPath, wantPath)
	}
}

func TestWorkflowExecutePassesEnvToRunner(t *testing.T) {
	wantEnv := map[string]string{"TOKEN": "abc123"}
	var gotEnv map[string]string

	p, _ := NewWorkflow("sub.flow.yaml", wantEnv, func(_ context.Context, _ string, _ map[string]any, env map[string]string) (any, error) {
		gotEnv = env
		return nil, nil
	})

	if _, err := p.Execute(context.Background(), nil); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotEnv["TOKEN"] != "abc123" {
		t.Errorf("runner received env TOKEN=%q, want abc123", gotEnv["TOKEN"])
	}
}

func TestWorkflowExecuteNilInputPassesEmptyMap(t *testing.T) {
	var gotInputs map[string]any

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, inputs map[string]any, _ map[string]string) (any, error) {
		gotInputs = inputs
		return nil, nil
	})

	if _, err := p.Execute(context.Background(), nil); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotInputs == nil {
		t.Error("runner received nil inputs, want empty map")
	}
	if len(gotInputs) != 0 {
		t.Errorf("runner received %d input(s), want 0", len(gotInputs))
	}
}

func TestWorkflowExecuteMapInputPassedDirectly(t *testing.T) {
	wantInputs := map[string]any{"repo": "myrepo", "branch": "main"}
	var gotInputs map[string]any

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, inputs map[string]any, _ map[string]string) (any, error) {
		gotInputs = inputs
		return nil, nil
	})

	if _, err := p.Execute(context.Background(), wantInputs); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotInputs["repo"] != "myrepo" || gotInputs["branch"] != "main" {
		t.Errorf("runner received inputs %v, want %v", gotInputs, wantInputs)
	}
}

func TestWorkflowExecuteJSONStringInputUnmarshalled(t *testing.T) {
	var gotInputs map[string]any

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, inputs map[string]any, _ map[string]string) (any, error) {
		gotInputs = inputs
		return nil, nil
	})

	if _, err := p.Execute(context.Background(), `{"repo":"myrepo","branch":"main"}`); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotInputs["repo"] != "myrepo" {
		t.Errorf("inputs[repo] = %v, want myrepo", gotInputs["repo"])
	}
}

func TestWorkflowExecutePlainStringInputWrapped(t *testing.T) {
	var gotInputs map[string]any

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, inputs map[string]any, _ map[string]string) (any, error) {
		gotInputs = inputs
		return nil, nil
	})

	if _, err := p.Execute(context.Background(), "hello"); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotInputs["value"] != "hello" {
		t.Errorf("inputs[value] = %v, want hello", gotInputs["value"])
	}
}

func TestWorkflowExecuteReturnsChildOutput(t *testing.T) {
	want := map[string]any{"score": float64(42)}

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return want, nil
	})

	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("Execute() returned %T, want map[string]any", out)
	}
	if m["score"] != float64(42) {
		t.Errorf("output score = %v, want 42", m["score"])
	}
}

func TestWorkflowExecuteRunnerErrorPropagated(t *testing.T) {
	runnerErr := errors.New("child workflow failed")

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(_ context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, runnerErr
	})

	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if !errors.Is(err, runnerErr) {
		t.Errorf("Execute() error = %v, expected to wrap %v", err, runnerErr)
	}
}

func TestWorkflowExecuteContextCancelledPropagated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p, _ := NewWorkflow("sub.flow.yaml", nil, func(ctx context.Context, _ string, _ map[string]any, _ map[string]string) (any, error) {
		return nil, ctx.Err()
	})

	_, err := p.Execute(ctx, nil)
	if err == nil {
		t.Fatal("Execute() expected error on cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Execute() error = %v, expected context.Canceled", err)
	}
}

// ----- inputToWorkflowMap -----

func TestInputToWorkflowMapNil(t *testing.T) {
	m, err := inputToWorkflowMap(nil)
	if err != nil {
		t.Fatalf("inputToWorkflowMap(nil) error: %v", err)
	}
	if m == nil || len(m) != 0 {
		t.Errorf("inputToWorkflowMap(nil) = %v, want empty map", m)
	}
}

func TestInputToWorkflowMapStringMap(t *testing.T) {
	in := map[string]any{"a": "1", "b": float64(2)}
	m, err := inputToWorkflowMap(in)
	if err != nil {
		t.Fatalf("inputToWorkflowMap error: %v", err)
	}
	if m["a"] != "1" || m["b"] != float64(2) {
		t.Errorf("inputToWorkflowMap = %v, want %v", m, in)
	}
}

func TestInputToWorkflowMapJSONString(t *testing.T) {
	m, err := inputToWorkflowMap(`{"x":10}`)
	if err != nil {
		t.Fatalf("inputToWorkflowMap error: %v", err)
	}
	if m["x"] != float64(10) {
		t.Errorf("m[x] = %v, want 10", m["x"])
	}
}

func TestInputToWorkflowMapPlainString(t *testing.T) {
	m, err := inputToWorkflowMap("not-json")
	if err != nil {
		t.Fatalf("inputToWorkflowMap error: %v", err)
	}
	if m["value"] != "not-json" {
		t.Errorf("m[value] = %v, want not-json", m["value"])
	}
}

func TestInputToWorkflowMapScalar(t *testing.T) {
	m, err := inputToWorkflowMap(42)
	if err != nil {
		t.Fatalf("inputToWorkflowMap error: %v", err)
	}
	// A scalar int cannot be represented as a map, so it is wrapped as {"value": v}.
	// The original Go value is preserved (not JSON-coerced to float64).
	if m["value"] != 42 {
		t.Errorf("m[value] = %v (%T), want int(42)", m["value"], m["value"])
	}
}

func TestInputToWorkflowMapBool(t *testing.T) {
	m, err := inputToWorkflowMap(true)
	if err != nil {
		t.Fatalf("inputToWorkflowMap error: %v", err)
	}
	if m["value"] != true {
		t.Errorf("m[value] = %v, want true", m["value"])
	}
}
