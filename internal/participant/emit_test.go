package participant

import (
	"context"
	"testing"

	"github.com/duckflux/runner/internal/model"
)

func TestEmitExecute_ReturnsExpectedResult(t *testing.T) {
	def := model.Participant{
		Type:    model.ParticipantTypeEmit,
		Event:   "test.event",
		Payload: map[string]any{"k": "v"},
		Ack:     true,
	}

	e := NewEmit(def)
	res, err := e.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("emit Execute error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %#v", res)
	}
	if m["event"] != "test.event" {
		t.Fatalf("expected event 'test.event', got %v", m["event"])
	}
	if ack, _ := m["ack"].(bool); !ack {
		t.Fatalf("expected ack true")
	}
}
