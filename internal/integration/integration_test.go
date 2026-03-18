// Package integration contains end-to-end tests that wire the parser, engine,
// and participant implementations together with real exec commands and a local
// HTTP test server. These tests exercise the full execution pipeline without
// mocks.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckflux/runner/internal/engine"
	"github.com/duckflux/runner/internal/parser"
	"github.com/duckflux/runner/internal/participant"
)

// runWorkflow is a convenience wrapper that executes with a background context.
func runWorkflow(t *testing.T, yaml string, inputs map[string]any) (any, error) {
	return runWorkflowWithContext(t, context.Background(), yaml, inputs)
}

// runWorkflowWithContext parses a YAML workflow string, builds the participant
// registry, and executes it with the provided context.
func runWorkflowWithContext(t *testing.T, ctx context.Context, yaml string, inputs map[string]any) (any, error) {
	t.Helper()
	wf, err := parser.Parse(strings.NewReader(yaml))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	env := map[string]string{}
	var runnerFn participant.SubWorkflowRunnerFunc
	runnerFn = func(ctx context.Context, path string, wfInputs map[string]any, wfEnv map[string]string) (any, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening sub-workflow %q: %w", path, err)
		}
		defer f.Close()

		subWF, err := parser.Parse(f)
		if err != nil {
			return nil, fmt.Errorf("parsing sub-workflow %q: %w", path, err)
		}

		reg, err := participant.BuildRegistry(subWF, wfEnv, runnerFn)
		if err != nil {
			return nil, fmt.Errorf("building registry for sub-workflow %q: %w", path, err)
		}

		return engine.Run(ctx, subWF, wfInputs, wfEnv, reg)
	}

	reg, err := participant.BuildRegistry(wf, env, runnerFn)
	if err != nil {
		return nil, fmt.Errorf("build registry: %w", err)
	}

	return engine.Run(ctx, wf, inputs, env, reg)
}

// runWorkflowFile parses a workflow from a file path and executes it.
func runWorkflowFile(t *testing.T, path string, inputs map[string]any) (any, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	return runWorkflow(t, string(data), inputs)
}

// examplesDir returns the absolute path to the examples/ directory.
func examplesDir(t *testing.T) string {
	t.Helper()
	// From internal/integration/ go up two levels to the module root.
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatalf("resolving examples dir: %v", err)
	}
	return dir
}

func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// ── exec participant ─────────────────────────────────────────────────────────

func TestExecSingleCommand(t *testing.T) {
	yaml := `
id: exec-single
participants:
  greet:
    type: exec
    run: echo "hello world"
flow:
  - greet
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "hello world") {
		t.Errorf("output = %q, want string containing 'hello world'", s)
	}
}

func TestExecInputPassedToStdin(t *testing.T) {
	yaml := `
id: exec-stdin
inputs:
  message:
    type: string
    default: "default-message"
participants:
  echo:
    type: exec
    run: cat
    input: workflow.inputs["message"]
flow:
  - echo
`
	out, err := runWorkflow(t, yaml, map[string]any{"message": "piped-value"})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "piped-value") {
		t.Errorf("output = %q, want string containing 'piped-value'", s)
	}
}

func TestExecJSONOutputAutoDetected(t *testing.T) {
	yaml := `
id: exec-json
participants:
  produce:
    type: exec
    run: printf '{"key":"value","count":42}'
flow:
  - produce
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, want value", m["key"])
	}
}

func TestExecMultipleSequentialSteps(t *testing.T) {
	yaml := `
id: exec-sequential
participants:
  first:
    type: exec
    run: echo "first"
  second:
    type: exec
    run: echo "second"
flow:
  - first
  - second
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	// Last step's output is returned when no explicit output is set.
	if !strings.Contains(s, "second") {
		t.Errorf("output = %q, want string containing 'second'", s)
	}
}

func TestExecOnErrorSkipContinues(t *testing.T) {
	yaml := `
id: exec-skip
participants:
  fail:
    type: exec
    run: exit 1
    onError: skip
  succeed:
    type: exec
    run: echo "after skip"
flow:
  - fail
  - succeed
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "after skip") {
		t.Errorf("output = %q, want string containing 'after skip'", s)
	}
}

func TestExecOnErrorFailAbortsWorkflow(t *testing.T) {
	yaml := `
id: exec-fail
participants:
  fail:
    type: exec
    run: exit 1
    onError: fail
  unreachable:
    type: exec
    run: echo "should not run"
flow:
  - fail
  - unreachable
`
	_, err := runWorkflow(t, yaml, nil)
	if err == nil {
		t.Fatal("Run() expected error for failing step with onError=fail, got nil")
	}
}

func TestExecCWDFromCLIBase(t *testing.T) {
	baseDir := t.TempDir()
	yaml := `
id: exec-cwd-cli
participants:
  where:
    type: exec
    run: pwd
flow:
  - where
`
	ctx := engine.WithBaseCWD(context.Background(), baseDir)
	out, err := runWorkflowWithContext(t, ctx, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	gotPath := canonicalPath(strings.TrimSpace(got))
	wantPath := canonicalPath(baseDir)
	if gotPath != wantPath {
		t.Errorf("pwd = %q, want %q", gotPath, wantPath)
	}
}

func TestExecCWDDefaultsOverrideCLIBase(t *testing.T) {
	baseDir := t.TempDir()
	defaultsDir := t.TempDir()
	yaml := fmt.Sprintf(`
id: exec-cwd-defaults
defaults:
  cwd: %q
participants:
  where:
    type: exec
    run: pwd
flow:
  - where
`, defaultsDir)
	ctx := engine.WithBaseCWD(context.Background(), baseDir)
	out, err := runWorkflowWithContext(t, ctx, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got := strings.TrimSpace(out.(string))
	if canonicalPath(got) != canonicalPath(defaultsDir) {
		t.Errorf("pwd = %q, want defaults.cwd %q", canonicalPath(got), canonicalPath(defaultsDir))
	}
}

func TestExecCWDParticipantOverrideDefaultsAndCLI(t *testing.T) {
	baseDir := t.TempDir()
	defaultsDir := t.TempDir()
	participantDir := t.TempDir()
	yaml := fmt.Sprintf(`
id: exec-cwd-participant
defaults:
  cwd: %q
participants:
  where:
    type: exec
    run: pwd
    cwd: %q
flow:
  - where
`, defaultsDir, participantDir)
	ctx := engine.WithBaseCWD(context.Background(), baseDir)
	out, err := runWorkflowWithContext(t, ctx, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got := strings.TrimSpace(out.(string))
	if canonicalPath(got) != canonicalPath(participantDir) {
		t.Errorf("pwd = %q, want participant.cwd %q", canonicalPath(got), canonicalPath(participantDir))
	}
}

func TestExecCWDParticipantSupportsCELVariables(t *testing.T) {
	cwdDir := t.TempDir()
	yaml := `
id: exec-cwd-cel
inputs:
  dir:
    type: string
participants:
  where:
    type: exec
    run: pwd
    cwd: workflow.inputs["dir"]
flow:
  - where
`
	out, err := runWorkflow(t, yaml, map[string]any{"dir": cwdDir})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got := strings.TrimSpace(out.(string))
	if canonicalPath(got) != canonicalPath(cwdDir) {
		t.Errorf("pwd = %q, want input dir %q", canonicalPath(got), canonicalPath(cwdDir))
	}
}

// ── loop ─────────────────────────────────────────────────────────────────────

func TestLoopMaxIterations(t *testing.T) {
	yaml := `
id: loop-max
participants:
  counter:
    type: exec
    run: echo "tick"
  done:
    type: exec
    run: echo "finished"
flow:
  - loop:
      max: 3
      steps:
        - counter
  - done
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// The step after the loop is the last executed step.
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "finished") {
		t.Errorf("output = %q, want string containing 'finished'", s)
	}
}

func TestLoopUntilConditionExits(t *testing.T) {
	// Each iteration the step outputs "done". The until expression
	// checks that output and should exit after the first iteration.
	yaml := `
id: loop-until
participants:
  worker:
    type: exec
    run: echo "done"
  after:
    type: exec
    run: echo "loop-exited"
flow:
  - loop:
      until: worker.output == "done\n"
      max: 10
      steps:
        - worker
  - after
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "loop-exited") {
		t.Errorf("output = %q, want string containing 'loop-exited'", s)
	}
}

func TestLoopWithWhenGuardInsideBody(t *testing.T) {
	// The "always" step runs every iteration; the "conditional" step only runs
	// when the when guard is true (which we set to always false here so we can
	// verify it is skipped without producing an error).
	yaml := `
id: loop-when
participants:
  always:
    type: exec
    run: echo "always"
  conditional:
    type: exec
    run: echo "conditional"
flow:
  - loop:
      max: 2
      steps:
        - always
        - conditional:
            when: "false"
`
	_, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

// ── parallel ─────────────────────────────────────────────────────────────────

func TestParallelStepsAllExecute(t *testing.T) {
	yaml := `
id: parallel-exec
participants:
  stepA:
    type: exec
    run: echo "A"
  stepB:
    type: exec
    run: echo "B"
  stepC:
    type: exec
    run: echo "C"
  summary:
    type: exec
    run: echo "done"
flow:
  - parallel:
      - stepA
      - stepB
      - stepC
  - summary
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "done") {
		t.Errorf("output = %q, want string containing 'done'", s)
	}
}

func TestParallelFailurePropagates(t *testing.T) {
	yaml := `
id: parallel-fail
participants:
  ok:
    type: exec
    run: echo "ok"
  bad:
    type: exec
    run: exit 1
flow:
  - parallel:
      - ok
      - bad
`
	_, err := runWorkflow(t, yaml, nil)
	if err == nil {
		t.Fatal("Run() expected error from parallel failure, got nil")
	}
}

// ── conditional (if/then/else) ────────────────────────────────────────────────

func TestIfThenBranchTaken(t *testing.T) {
	yaml := `
id: if-then
participants:
  produce:
    type: exec
    run: printf '{"score":9}'
  high:
    type: exec
    run: echo "high score"
  low:
    type: exec
    run: echo "low score"
flow:
  - produce
  - if:
      condition: "produce.output.score > 7"
      then:
        - high
      else:
        - low
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "high score") {
		t.Errorf("output = %q, want string containing 'high score'", s)
	}
}

func TestIfElseBranchTaken(t *testing.T) {
	yaml := `
id: if-else
participants:
  produce:
    type: exec
    run: printf '{"score":3}'
  high:
    type: exec
    run: echo "high score"
  low:
    type: exec
    run: echo "low score"
flow:
  - produce
  - if:
      condition: "produce.output.score > 7"
      then:
        - high
      else:
        - low
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "low score") {
		t.Errorf("output = %q, want string containing 'low score'", s)
	}
}

// ── http participant ──────────────────────────────────────────────────────────

func TestHTTPGetFromLocalServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "pong")
	}))
	defer ts.Close()

	yaml := fmt.Sprintf(`
id: http-get
participants:
  ping:
    type: http
    url: %s
    method: GET
flow:
  - ping
`, ts.URL)

	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out != "pong" {
		t.Errorf("output = %q, want pong", out)
	}
}

func TestHTTPPostWithBodyToLocalServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"received":true}`)
	}))
	defer ts.Close()

	yaml := fmt.Sprintf(`
id: http-post
participants:
  send:
    type: http
    url: %s
    method: POST
    body: "hello"
flow:
  - send
`, ts.URL)

	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["received"] != true {
		t.Errorf("received = %v, want true", m["received"])
	}
}

func TestHTTPJSONResponseParsed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok","items":["a","b"]}`)
	}))
	defer ts.Close()

	yaml := fmt.Sprintf(`
id: http-json
participants:
  fetch:
    type: http
    url: %s
    method: GET
flow:
  - fetch
`, ts.URL)

	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
}

func TestHTTPOutputUsedInSubsequentExecStep(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"greeting":"hello-from-server"}`)
	}))
	defer ts.Close()

	yaml := fmt.Sprintf(`
id: http-then-exec
participants:
  fetch:
    type: http
    url: %s
    method: GET
  process:
    type: exec
    run: cat
    input:
      msg: fetch.output.greeting
flow:
  - fetch
  - process
`, ts.URL)

	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s := fmt.Sprintf("%v", out)
	if !strings.Contains(s, "hello-from-server") {
		t.Errorf("output = %q, want string containing 'hello-from-server'", s)
	}
}

func TestHTTPOnErrorSkipContinues(t *testing.T) {
	// Use an unreachable port — the http participant will fail.
	yaml := `
id: http-skip
participants:
  bad:
    type: http
    url: http://127.0.0.1:1
    method: GET
    onError: skip
  after:
    type: exec
    run: echo "continued"
flow:
  - bad
  - after
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "continued") {
		t.Errorf("output = %q, want string containing 'continued'", s)
	}
}

func TestHTTPDynamicFieldsFromCEL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			w,
			`{"method":"%s","token":"%s","msg":"%v","literal":"%v"}`,
			r.Method,
			r.Header.Get("X-Token"),
			body["msg"],
			body["literal"],
		)
	}))
	defer ts.Close()

	yaml := `
id: http-dynamic
inputs:
  endpoint:
    type: string
  token:
    type: string
  msg:
    type: string
participants:
  call:
    type: http
    url: workflow.inputs["endpoint"]
    method: POST
    headers:
      X-Token: workflow.inputs["token"]
    body:
      msg: workflow.inputs["msg"]
      literal: static-value
flow:
  - call
`

	out, err := runWorkflow(t, yaml, map[string]any{
		"endpoint": ts.URL,
		"token":    "abc123",
		"msg":      "hello-dynamic",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["method"] != "POST" {
		t.Errorf("method = %v, want POST", m["method"])
	}
	if m["token"] != "abc123" {
		t.Errorf("token = %v, want abc123", m["token"])
	}
	if m["msg"] != "hello-dynamic" {
		t.Errorf("msg = %v, want hello-dynamic", m["msg"])
	}
	if m["literal"] != "static-value" {
		t.Errorf("literal = %v, want static-value", m["literal"])
	}
}

// ── v0.3 chain semantics ──────────────────────────────────────────────────────

func TestV03_AnonymousInlineMinimalWorkflow(t *testing.T) {
	yaml := `
flow:
  - type: exec
    run: echo "anonymous-ok"
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "anonymous-ok") {
		t.Errorf("output = %q, want string containing 'anonymous-ok'", s)
	}
}

func TestV03_ImplicitChainSequential(t *testing.T) {
	// Step 1 outputs JSON, step 2 receives it via chain (cat reads from stdin).
	yaml := `
participants:
  produce:
    type: exec
    run: echo "chained-value"
  consume:
    type: exec
    run: cat
flow:
  - produce
  - consume
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "chained-value") {
		t.Errorf("output = %q, want string containing 'chained-value'", s)
	}
}

func TestV03_ChainMapMergePrecedence(t *testing.T) {
	// Chain provides a map; explicit input provides overlapping keys; explicit wins.
	yaml := `
participants:
  produce:
    type: exec
    run: printf '{"a":"from-chain","b":"from-chain"}'
  consume:
    type: exec
    run: cat
    input:
      a: '"from-explicit"'
flow:
  - produce
  - consume
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["a"] != "from-explicit" {
		t.Errorf("a = %v, want from-explicit", m["a"])
	}
	if m["b"] != "from-chain" {
		t.Errorf("b = %v, want from-chain", m["b"])
	}
}

func TestV03_ChainIncompatibleTypesError(t *testing.T) {
	// Chain is a map, explicit input is a string → incompatible types error per spec §5.7.
	yaml := `
participants:
  produce:
    type: exec
    run: printf '{"key":"value"}'
  consume:
    type: exec
    run: cat
    input: '"override-string"'
flow:
  - produce
  - consume
`
	_, err := runWorkflow(t, yaml, nil)
	if err == nil {
		t.Fatal("expected incompatible chain merge error, got nil")
	}
	if !strings.Contains(err.Error(), "incompatible types") {
		t.Errorf("error = %q, want string containing 'incompatible types'", err.Error())
	}
}

func TestV03_IfPassThroughWhenFalseNoElse(t *testing.T) {
	// If condition is false, no else branch → chain unchanged.
	yaml := `
participants:
  produce:
    type: exec
    run: echo "original-chain"
  unreachable:
    type: exec
    run: echo "should-not-appear"
flow:
  - produce
  - if:
      condition: "false"
      then:
        - unreachable
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "original-chain") {
		t.Errorf("output = %q, want string containing 'original-chain'", s)
	}
}

func TestV03_ParallelReturnsOrderedArray(t *testing.T) {
	yaml := `
participants:
  stepA:
    type: exec
    run: echo "A"
  stepB:
    type: exec
    run: echo "B"
flow:
  - parallel:
      - stepA
      - stepB
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	arr, ok := out.([]any)
	if !ok {
		t.Fatalf("output type = %T, want []any (parallel outputs ordered array)", out)
	}
	if len(arr) != 2 {
		t.Fatalf("output len = %d, want 2", len(arr))
	}
	// Check that A and B are in declaration order.
	sA, _ := arr[0].(string)
	sB, _ := arr[1].(string)
	if !strings.Contains(sA, "A") {
		t.Errorf("arr[0] = %q, want 'A'", sA)
	}
	if !strings.Contains(sB, "B") {
		t.Errorf("arr[1] = %q, want 'B'", sB)
	}
}

func TestV03_WorkflowInputsViaWorkflowNamespace(t *testing.T) {
	yaml := `
inputs:
  name:
    type: string
participants:
  greet:
    type: exec
    run: cat
    input: workflow.inputs["name"]
flow:
  - greet
`
	out, err := runWorkflow(t, yaml, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "world") {
		t.Errorf("output = %q, want string containing 'world'", s)
	}
}

func TestV03_NamedInlineUniquenessConflict(t *testing.T) {
	yaml := `
participants:
  myStep:
    type: exec
    run: echo "named"
flow:
  - type: exec
    as: myStep
    run: echo "inline"
`
	_, err := runWorkflow(t, yaml, nil)
	if err == nil {
		t.Fatal("expected validation error for duplicate inline name, got nil")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error = %q, want string containing 'conflicts'", err.Error())
	}
}

func TestV03_EmptyFlowRejected(t *testing.T) {
	yaml := `
flow: []
`
	_, err := runWorkflow(t, yaml, nil)
	if err == nil {
		t.Fatal("expected validation error for empty flow, got nil")
	}
}

func TestV03_AnonymousInlineInControlFlow(t *testing.T) {
	yaml := `
flow:
  - if:
      condition: "true"
      then:
        - type: exec
          run: echo "anon-in-if"
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "anon-in-if") {
		t.Errorf("output = %q, want string containing 'anon-in-if'", s)
	}
}

func TestV03_DefaultOutputIsFinalChain(t *testing.T) {
	// Without explicit output, the final chain value is returned.
	yaml := `
participants:
  step1:
    type: exec
    run: echo "step1-out"
  step2:
    type: exec
    run: echo "final-chain"
flow:
  - step1
  - step2
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "final-chain") {
		t.Errorf("output = %q, want string containing 'final-chain'", s)
	}
}

// ── output mapping ────────────────────────────────────────────────────────────

func TestExplicitOutputExpression(t *testing.T) {
	yaml := `
id: output-expr
participants:
  produce:
    type: exec
    run: printf '{"result":"computed"}'
flow:
  - produce
output:
  value: produce.output.result
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["value"] != "computed" {
		t.Errorf("output.value = %v, want computed", m["value"])
	}
}

func TestExplicitOutputMap(t *testing.T) {
	yaml := `
id: output-map
participants:
  produce:
    type: exec
    run: printf '{"code":"abc","status":"ok"}'
flow:
  - produce
output:
  myCode: produce.output.code
  myStatus: produce.output.status
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["myCode"] != "abc" {
		t.Errorf("myCode = %v, want abc", m["myCode"])
	}
	if m["myStatus"] != "ok" {
		t.Errorf("myStatus = %v, want ok", m["myStatus"])
	}
}

func TestExplicitOutputScalarExpression(t *testing.T) {
	yaml := `
id: output-scalar
participants:
  produce:
    type: exec
    run: printf '{"result":"only-value"}'
flow:
  - produce
output: produce.output.result
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out != "only-value" {
		t.Errorf("output = %v, want only-value", out)
	}
}

// ── exec + http combined ──────────────────────────────────────────────────────

func TestExecAndHTTPCombined(t *testing.T) {
	// HTTP server returns a JSON payload; an exec step extracts a field via shell.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"version":"1.2.3"}`)
	}))
	defer ts.Close()

	yaml := fmt.Sprintf(`
id: combined
participants:
  getVersion:
    type: http
    url: %s
    method: GET
  printVersion:
    type: exec
    run: cat
    input:
      ver: getVersion.output.version
flow:
  - getVersion
  - printVersion
`, ts.URL)

	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s := fmt.Sprintf("%v", out)
	if !strings.Contains(s, "1.2.3") {
		t.Errorf("output = %q, want string containing '1.2.3'", s)
	}
}

// ── example files ─────────────────────────────────────────────────────────────

// TestExampleMinimal parses and executes examples/minimal.flow.yaml end-to-end.
func TestExampleMinimal(t *testing.T) {
	path := filepath.Join(examplesDir(t), "minimal.flow.yaml")
	out, err := runWorkflowFile(t, path, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "Hello") {
		t.Errorf("output = %q, want string containing 'Hello'", s)
	}
}

// TestExampleLoop parses and executes examples/loop.flow.yaml end-to-end.
func TestExampleLoop(t *testing.T) {
	path := filepath.Join(examplesDir(t), "loop.flow.yaml")
	_, err := runWorkflowFile(t, path, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

// TestExampleParallel parses and executes examples/parallel.flow.yaml end-to-end.
func TestExampleParallel(t *testing.T) {
	path := filepath.Join(examplesDir(t), "parallel.flow.yaml")
	out, err := runWorkflowFile(t, path, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "done") {
		t.Errorf("output = %q, want string containing 'done'", s)
	}
}

// TestExampleCodeReview parses and executes examples/code-review.flow.yaml using a
// local HTTP test server in place of the external webhook URLs. The example file
// uses onError: skip for the notify steps, so the test creates a local server and
// rewrites the URLs in the YAML before executing.
func TestExampleCodeReview(t *testing.T) {
	notifyCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifyCalled = true
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	rawData, err := os.ReadFile(filepath.Join(examplesDir(t), "code-review.flow.yaml"))
	if err != nil {
		t.Fatalf("reading example: %v", err)
	}

	// Replace placeholder URL with the local test server URL.
	yaml := strings.ReplaceAll(string(rawData), "http://localhost:0", ts.URL)

	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want map[string]any", out)
	}
	if m["approved"] != true {
		t.Errorf("approved = %v, want true", m["approved"])
	}
	if !notifyCalled {
		t.Error("expected HTTP notify endpoint to be called")
	}

	// Verify the score field is present in the output.
	if _, hasScore := m["score"]; !hasScore {
		t.Error("output missing 'score' field")
	}
	if m["testResult"] != "success" {
		t.Errorf("testResult = %v, want success", m["testResult"])
	}
	if m["lintResult"] != "success" {
		t.Errorf("lintResult = %v, want success", m["lintResult"])
	}
}
