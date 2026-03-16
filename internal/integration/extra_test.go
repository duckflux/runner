package integration

import (
	"testing"

	"strings"
)

func TestIntegration_WaitSleepMode(t *testing.T) {
	yaml := `participants:
  greeter:
    type: exec
    run: echo done
flow:
  - wait:
      timeout: 10ms
  - greeter
output: greeter.output
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
		t.Errorf("output = %q, want contains 'done'", s)
	}
}

func TestIntegration_InlineParticipant(t *testing.T) {
	yaml := `flow:
  - type: exec
    as: inline
    run: echo inline
output: inline.output
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if !strings.Contains(s, "inline") {
		t.Errorf("output = %q, want contains 'inline'", s)
	}
}

func TestIntegration_EmitParticipant(t *testing.T) {
	yaml := `participants:
  emitter:
    type: emit
    event: test.ev
    payload:
      k: v
flow:
  - emitter
output: emitter.output.event
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if s != "test.ev" {
		t.Errorf("output = %q, want 'test.ev'", s)
	}
}

func TestIntegration_LoopAsRuns(t *testing.T) {
	yaml := `participants:
  p:
    type: exec
    run: echo hi
flow:
  - loop:
      as: attempt
      max: 2
      steps:
        - p
output: p.output
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	_, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
}

func TestIntegration_LoopAsAliasInWhen(t *testing.T) {
	yaml := `participants:
  p:
    type: exec
    run: echo hi
flow:
  - loop:
      as: attempt
      max: 2
      steps:
        - p:
            when: attempt.index > 0
output: p.status
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	if s != "success" {
		t.Fatalf("output = %q, want success", s)
	}
}

func TestIntegration_WaitEventFromEmit(t *testing.T) {
	yaml := `participants:
  emitter:
    type: emit
    event: approval.response
    payload:
      approved: true
flow:
  - emitter
  - wait:
      event: approval.response
      match: event.approved == true
      timeout: 1s
output: event.approved
`
	out, err := runWorkflow(t, yaml, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	b, ok := out.(bool)
	if !ok {
		t.Fatalf("output type = %T, want bool", out)
	}
	if !b {
		t.Fatalf("output = %v, want true", b)
	}
}
