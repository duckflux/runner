package parser

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// ----- Parse happy-path -----

func TestParseMinimalValid(t *testing.T) {
	src := `
id: test-workflow
participants:
  stepA:
    type: exec
    run: echo hello
flow:
  - stepA
`
	wf, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.ID != "test-workflow" {
		t.Errorf("ID = %q, want %q", wf.ID, "test-workflow")
	}
	if len(wf.Participants) != 1 {
		t.Fatalf("Participants len = %d, want 1", len(wf.Participants))
	}
	if len(wf.Flow) != 1 {
		t.Fatalf("Flow len = %d, want 1", len(wf.Flow))
	}
	if wf.Flow[0].Participant != "stepA" {
		t.Errorf("Flow[0].Participant = %q, want %q", wf.Flow[0].Participant, "stepA")
	}
}

func TestParseFullWorkflow(t *testing.T) {
	src := `
id: full-workflow
name: Full Workflow
version: "1.0"
defaults:
  timeout: 5m
  onError: fail
inputs:
  repoUrl:
    type: string
    required: true
participants:
  coder:
    type: agent
    model: claude-sonnet-4
    timeout: 15m
    onError: retry
    retry:
      max: 3
      backoff: 2s
      factor: 2
  reviewer:
    type: agent
    model: claude-sonnet-4
flow:
  - coder
  - loop:
      until: "reviewer.output.approved == true"
      max: 5
      steps:
        - coder
        - reviewer
output:
  approved: reviewer.output.approved
  code: coder.output.code
`
	wf, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.ID != "full-workflow" {
		t.Errorf("ID = %q, want full-workflow", wf.ID)
	}
	if wf.Defaults == nil || wf.Defaults.Timeout == nil {
		t.Fatal("Defaults.Timeout should not be nil")
	}
	if wf.Defaults.Timeout.Duration != 5*time.Minute {
		t.Errorf("Defaults.Timeout = %v, want 5m", wf.Defaults.Timeout.Duration)
	}
	if len(wf.Participants) != 2 {
		t.Fatalf("Participants len = %d, want 2", len(wf.Participants))
	}
	coder := wf.Participants["coder"]
	if coder.Retry == nil || coder.Retry.Max != 3 {
		t.Errorf("coder.Retry.Max = %v, want 3", coder.Retry)
	}
	if len(wf.Flow) != 2 {
		t.Fatalf("Flow len = %d, want 2", len(wf.Flow))
	}
	if wf.Flow[1].Loop == nil {
		t.Fatal("Flow[1].Loop should not be nil")
	}
	if wf.Output == nil || wf.Output.Map == nil {
		t.Fatal("Output.Map should not be nil")
	}
}

// ----- Parse error cases -----

func TestParseMissingID(t *testing.T) {
	src := `
participants:
  stepA:
    type: exec
    run: echo hello
flow:
  - stepA
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !isValidationError(err) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestParseMissingParticipants(t *testing.T) {
	src := `
id: no-participants
flow:
  - stepA
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for missing participants, got nil")
	}
	if !isValidationError(err) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestParseMissingFlow(t *testing.T) {
	src := `
id: no-flow
participants:
  stepA:
    type: exec
    run: echo hello
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for missing flow, got nil")
	}
	if !isValidationError(err) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
}

func TestParseInvalidYAMLSyntax(t *testing.T) {
	src := `id: [broken yaml`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseInvalidParticipantType(t *testing.T) {
	src := `
id: bad-type
participants:
  stepA:
    type: notavalidtype
flow:
  - stepA
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for invalid participant type, got nil")
	}
	if !isValidationError(err) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
}

// ----- ValidationError -----

func TestValidationErrorMessage(t *testing.T) {
	e := &ValidationError{Field: "/id", Message: "must be a string"}
	if e.Error() != "/id: must be a string" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestValidationErrorNoField(t *testing.T) {
	e := &ValidationError{Message: "something is wrong"}
	if e.Error() != "something is wrong" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestValidationErrorsError(t *testing.T) {
	ve := ValidationErrors{
		{Field: "/id", Message: "required"},
		{Field: "/flow", Message: "required"},
	}
	got := ve.Error()
	if !strings.Contains(got, "/id: required") {
		t.Errorf("Error() missing /id: required, got %q", got)
	}
	if !strings.Contains(got, "/flow: required") {
		t.Errorf("Error() missing /flow: required, got %q", got)
	}
}

// ----- helpers -----

func isValidationError(err error) bool {
	var ve ValidationErrors
	if errors.As(err, &ve) {
		return true
	}
	var sve *ValidationError
	return errors.As(err, &sve)
}
