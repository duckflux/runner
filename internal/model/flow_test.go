package model

import (
	"gopkg.in/yaml.v3"
	"testing"
	"time"
)

func TestFlowUnmarshal_WaitInlineLoopIf(t *testing.T) {
	data := `flow:
  - wait:
      timeout: 10ms

  - type: exec
    as: inlineExec
    run: echo hi

  - loop:
      as: attempt
      max: 2
      steps:
        - inlineExec

  - if:
      condition: 'true'
      then:
        - inlineExec
      else:
        - inlineExec
`

	var wf Workflow
	if err := yaml.Unmarshal([]byte(data), &wf); err != nil {
		t.Fatalf("unmarshal workflow: %v", err)
	}

	if len(wf.Flow) != 4 {
		t.Fatalf("expected 4 flow steps, got %d", len(wf.Flow))
	}

	// wait step
	if wf.Flow[0].Wait == nil {
		t.Fatalf("expected wait step at index 0")
	}
	if wf.Flow[0].Wait.Timeout == nil || wf.Flow[0].Wait.Timeout.Duration < 10*time.Millisecond {
		t.Fatalf("wait timeout not parsed correctly: %#v", wf.Flow[0].Wait.Timeout)
	}

	// inline participant
	if wf.Flow[1].InlineParticipant == nil {
		t.Fatalf("expected inline participant at index 1")
	}
	if wf.Flow[1].InlineParticipant.Type != ParticipantTypeExec {
		t.Fatalf("expected inline participant type exec, got %v", wf.Flow[1].InlineParticipant.Type)
	}
	if wf.Flow[1].InlineParticipant.As != "inlineExec" {
		t.Fatalf("expected inline participant as 'inlineExec', got %q", wf.Flow[1].InlineParticipant.As)
	}

	// loop.as
	if wf.Flow[2].Loop == nil {
		t.Fatalf("expected loop step at index 2")
	}
	if wf.Flow[2].Loop.As != "attempt" {
		t.Fatalf("expected loop.as 'attempt', got %q", wf.Flow[2].Loop.As)
	}

	// if condition
	if wf.Flow[3].If == nil {
		t.Fatalf("expected if step at index 3")
	}
	if wf.Flow[3].If.Condition != "true" {
		t.Fatalf("expected if.condition 'true', got %q", wf.Flow[3].If.Condition)
	}
}
