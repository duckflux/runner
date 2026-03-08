package engine

import (
	"context"
	"testing"
	"time"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
)

func TestRunWait_SleepMode(t *testing.T) {
	wf := &model.Workflow{}
	step := &model.WaitStep{Timeout: &model.Duration{Duration: 10 * time.Millisecond}}
	state := &cel.State{}
	celEnv, err := cel.NewEnv(wf)
	if err != nil {
		t.Fatalf("cel env: %v", err)
	}
	ctx := context.Background()
	if err := runWait(ctx, wf, step, state, celEnv, nil); err != nil {
		t.Fatalf("runWait sleep returned error: %v", err)
	}
}

func TestRunWait_PollingModeImmediateTrue(t *testing.T) {
	wf := &model.Workflow{}
	step := &model.WaitStep{Until: "true", Poll: &model.Duration{Duration: 1 * time.Millisecond}}
	state := &cel.State{}
	celEnv, err := cel.NewEnv(wf)
	if err != nil {
		t.Fatalf("cel env: %v", err)
	}
	ctx := context.Background()
	if err := runWait(ctx, wf, step, state, celEnv, nil); err != nil {
		t.Fatalf("runWait polling returned error: %v", err)
	}
}
