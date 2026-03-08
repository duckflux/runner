package parser

import (
	"strings"
	"testing"
)

func TestParse_InlineParticipant(t *testing.T) {
	yaml := `flow:
  - type: exec
    as: inline
    run: echo inline
`
	wf, err := Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(wf.Flow) != 1 || wf.Flow[0].InlineParticipant == nil {
		t.Fatalf("expected inline participant in flow")
	}
}

func TestParse_EmitParticipant(t *testing.T) {
	yaml := `participants:
  emitter:
    type: emit
    event: test.ev
flow:
  - emitter
`
	wf, err := Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if wf.Participants == nil {
		t.Fatalf("expected participants map")
	}
	if _, ok := wf.Participants["emitter"]; !ok {
		t.Fatalf("expected emitter participant")
	}
}

func TestParse_WaitStep(t *testing.T) {
	yaml := `flow:
  - wait:
      timeout: 10ms
`
	_, err := Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
}
