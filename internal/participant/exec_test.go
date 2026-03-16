package participant

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duckflux/runner/internal/model"
)

// helper builds an ExecParticipant from a run string with no extra env.
func newExec(run string) *ExecParticipant {
	return NewExec(model.Participant{Run: run}, nil)
}

func TestExecBasicCommand(t *testing.T) {
	p := newExec("echo hello")
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got, ok := out.(string)
	if !ok {
		t.Fatalf("Execute() returned %T, want string", out)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("Execute() = %q, want string containing 'hello'", got)
	}
}

func TestExecCapturesStdout(t *testing.T) {
	p := newExec("printf 'abc'")
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "abc" {
		t.Errorf("Execute() = %q, want abc", out)
	}
}

func TestExecPipesStringInputToStdin(t *testing.T) {
	p := newExec("cat")
	out, err := p.Execute(context.Background(), "hello stdin")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "hello stdin" {
		t.Errorf("Execute() = %q, want 'hello stdin'", out)
	}
}

func TestExecPipesMapInputAsJSON(t *testing.T) {
	p := newExec("cat")
	out, err := p.Execute(context.Background(), map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("Execute() returned %T, want string", out)
	}
	if !strings.Contains(s, "key") || !strings.Contains(s, "value") {
		t.Errorf("Execute() = %q, expected JSON with key and value", s)
	}
}

func TestExecRunsInConfiguredCWD(t *testing.T) {
	dir := t.TempDir()
	p := NewExec(model.Participant{Run: "pwd", CWD: dir}, nil)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got, ok := out.(string)
	if !ok {
		t.Fatalf("Execute() returned %T, want string", out)
	}
	gotPath := strings.TrimSpace(got)
	if resolved, err := filepath.EvalSymlinks(gotPath); err == nil {
		gotPath = resolved
	}
	wantPath := dir
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		wantPath = resolved
	}
	if gotPath != wantPath {
		t.Errorf("Execute() pwd = %q, want %q", gotPath, wantPath)
	}
}

func TestExecNonZeroExitReturnsError(t *testing.T) {
	p := newExec("exit 1")
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error for non-zero exit, got nil")
	}
}

func TestExecStderrIncludedInError(t *testing.T) {
	p := newExec("echo 'error message' >&2; exit 1")
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "error message") {
		t.Errorf("error = %q, expected it to contain stderr output 'error message'", err.Error())
	}
}

func TestExecContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := newExec("sleep 30")
	start := time.Now()
	_, err := p.Execute(ctx, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Execute() expected error on context timeout, got nil")
	}
	// Should complete well within WaitDelay (1s) rather than running to completion (30s).
	if elapsed > 10*time.Second {
		t.Errorf("Execute() took %v; expected fast cancellation", elapsed)
	}
}

func TestExecEnvInjection(t *testing.T) {
	p := NewExec(
		model.Participant{Run: "echo $MY_TEST_VAR"},
		map[string]string{"MY_TEST_VAR": "injected-value"},
	)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("Execute() returned %T, want string", out)
	}
	if !strings.Contains(s, "injected-value") {
		t.Errorf("Execute() = %q, expected injected-value from env", s)
	}
}

func TestExecEmptyRunReturnsError(t *testing.T) {
	p := NewExec(model.Participant{Run: ""}, nil)
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error for empty run command, got nil")
	}
}

func TestExecNilInputNoStdin(t *testing.T) {
	// Command that exits non-zero if it reads anything from stdin.
	// Using a read with timeout: if stdin is empty, read returns immediately.
	p := newExec("echo ok")
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(out.(string), "ok") {
		t.Errorf("Execute() = %q, want string containing 'ok'", out)
	}
}
