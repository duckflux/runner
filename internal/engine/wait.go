package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
	"github.com/duckflux/runner/internal/participant"
	gcel "github.com/google/cel-go/cel"
)

// runWait executes a wait step. It supports three modes:
// 1) Event mode: wait.Event != "" (stubbed in v1). Sets state.EventPayload.
// 2) Sleep mode: only Timeout is provided — sleeps until timeout or ctx cancel.
// 3) Polling mode: Until + Poll + Timeout — periodically evaluates Until.
func runWait(ctx context.Context, wf *model.Workflow, step *model.WaitStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) error {
	// Event mode: wait for matching event from internal hub.
	if step.Event != "" {
		var matchProg gcel.Program
		var err error
		if step.Match != "" {
			matchProg, err = celEnv.Compile(step.Match)
			if err != nil {
				return fmt.Errorf("wait: compiling match expression: %w", err)
			}
		}

		subID, ch := state.SubscribeEvents()
		defer state.UnsubscribeEvents(subID)

		var timeoutCh <-chan time.Time
		if step.Timeout != nil {
			timer := time.NewTimer(step.Timeout.Duration)
			defer timer.Stop()
			timeoutCh = timer.C
		}

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timeoutCh:
				return handleWaitTimeout(ctx, wf, step, state, celEnv, reg)
			case evt := <-ch:
				if evt.Name != step.Event {
					continue
				}
				state.EventPayload = evt.Payload
				state.Now = time.Now().UTC()
				if matchProg == nil {
					return nil
				}
				result, err := celEnv.Eval(matchProg, state)
				if err != nil {
					return fmt.Errorf("wait: evaluating match expression: %w", err)
				}
				cond, ok := result.(bool)
				if !ok {
					return fmt.Errorf("wait: match expression must evaluate to bool, got %T", result)
				}
				if cond {
					return nil
				}
			}
		}
	}

	// Sleep mode: only timeout specified
	if (step.Poll == nil || step.Until == "") && step.Timeout != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(step.Timeout.Duration):
			return nil
		}
	}

	// Polling mode: requires Until expression and Poll interval
	if step.Until != "" && step.Poll != nil {
		// Use compile/eval loop until condition true or timeout
		prog, err := celEnv.Compile(step.Until)
		if err != nil {
			return fmt.Errorf("wait: compiling until expression: %w", err)
		}

		deadline := time.Time{}
		if step.Timeout != nil {
			deadline = time.Now().Add(step.Timeout.Duration)
		}

		ticker := time.NewTicker(step.Poll.Duration)
		defer ticker.Stop()

		for {
			// Evaluate once immediately before waiting for the first tick
			state.Now = time.Now().UTC()
			result, err := celEnv.Eval(prog, state)
			if err != nil {
				return fmt.Errorf("wait: evaluating until expression: %w", err)
			}
			if cond, ok := result.(bool); ok && cond {
				return nil
			}

			// Check timeout
			if !deadline.IsZero() && time.Now().After(deadline) {
				return handleWaitTimeout(ctx, wf, step, state, celEnv, reg)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				continue
			}
		}
	}

	return fmt.Errorf("wait: invalid wait configuration")
}

func handleWaitTimeout(ctx context.Context, wf *model.Workflow, step *model.WaitStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) error {
	switch step.OnTimeout {
	case "", "fail":
		return fmt.Errorf("wait: timeout reached")
	case "skip":
		return nil
	default:
		if err := runParticipantStep(ctx, wf, step.OnTimeout, &model.ParticipantOverrideStep{OnError: "fail"}, state, celEnv, reg); err != nil {
			return fmt.Errorf("wait: onTimeout redirect failed: %w", err)
		}
		return nil
	}
}
