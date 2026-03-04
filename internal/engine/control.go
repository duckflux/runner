package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
	"github.com/duckflux/runner/internal/participant"
)

// runLoop executes a loop step, repeating the body steps until the until
// condition evaluates to true or the max iteration count is reached. Both
// until and max may be set simultaneously; the loop stops at whichever
// condition triggers first.
//
// The loop context (loop.index, loop.iteration, loop.first, loop.last) is set
// on state before each iteration and restored to its previous value when the
// loop exits, enabling correct semantics for nested loops.
func runLoop(ctx context.Context, wf *model.Workflow, step *model.LoopStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) error {
	// Pre-compile the until expression once if provided.
	var hasUntil bool
	if step.Until != "" {
		hasUntil = true
	}

	// Safety guard: without either until or max the loop would be infinite.
	// The semantic validator catches this at parse time; guard here as a safety net.
	if !hasUntil && step.Max == 0 {
		return fmt.Errorf("loop: neither until nor max specified; aborting to prevent infinite loop")
	}

	// Save and restore the outer loop context to support nested loops.
	prevLoop := state.Loop
	defer func() { state.Loop = prevLoop }()

	var iteration int64
	for {
		iteration++
		index := iteration - 1

		// Determine in advance whether this is the last iteration — only possible
		// when max is set. For until-only loops, last is resolved after the body runs.
		isLast := step.Max > 0 && int(iteration) == step.Max

		state.Loop = &cel.LoopContext{
			Index:     index,
			Iteration: iteration,
			First:     iteration == 1,
			Last:      isLast,
		}

		// Execute the loop body steps.
		if _, err := runSequential(ctx, wf, step.Steps, state, celEnv, reg); err != nil {
			return err
		}

		// Evaluate the until condition after the body has run.
		if hasUntil {
			prog, err := celEnv.Compile(step.Until)
			if err != nil {
				return fmt.Errorf("loop: compiling until expression: %w", err)
			}
			result, err := celEnv.Eval(prog, state)
			if err != nil {
				return fmt.Errorf("loop: evaluating until expression: %w", err)
			}
			if done, _ := result.(bool); done {
				break
			}
		}

		// Stop if the max iteration count has been reached.
		if step.Max > 0 && int(iteration) >= step.Max {
			break
		}
	}
	return nil
}

// runParallel executes all branches of a parallel step concurrently using one
// goroutine per participant. A derived cancellable context is shared across all
// goroutines: if any branch fails, cancel is called so that still-running
// branches are signalled to stop. The first error encountered is returned.
// Writes to state.Steps are made thread-safely via state.SetStep.
func runParallel(ctx context.Context, wf *model.Workflow, step *model.ParallelStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make([]error, len(step.Steps))
	var wg sync.WaitGroup

	for i, name := range step.Steps {
		wg.Add(1)
		go func(idx int, participantName string) {
			defer wg.Done()
			if err := runParticipantStep(ctx, wf, participantName, nil, state, celEnv, reg); err != nil {
				errs[idx] = err
				cancel() // signal all other branches to stop
			}
		}(i, name)
	}

	wg.Wait()

	// Return the first non-nil error.
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// runIf evaluates the CEL condition and executes either the then or the else
// branch. It returns the name of the last participant executed (if any), which
// may propagate up as the implicit workflow output.
func runIf(ctx context.Context, wf *model.Workflow, step *model.IfStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) (string, error) {
	prog, err := celEnv.Compile(step.Condition)
	if err != nil {
		return "", fmt.Errorf("if: compiling condition: %w", err)
	}
	result, err := celEnv.Eval(prog, state)
	if err != nil {
		return "", fmt.Errorf("if: evaluating condition: %w", err)
	}

	if cond, _ := result.(bool); cond {
		return runSequential(ctx, wf, step.Then, state, celEnv, reg)
	}
	if len(step.Else) > 0 {
		return runSequential(ctx, wf, step.Else, state, celEnv, reg)
	}
	return "", nil
}
