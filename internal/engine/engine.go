package engine

import (
	"context"
	"fmt"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
	"github.com/duckflux/runner/internal/participant"
)

// Run parses, initialises state for, and executes a workflow definition.
// inputs contains the caller-supplied workflow input values (may be nil).
// env contains process environment variables made available as env.* (may be nil).
// reg maps each participant name to its Participant implementation.
//
// The returned value is the resolved workflow output (a plain value, a
// map[string]any, or nil when no output is defined and no steps succeeded).
func Run(ctx context.Context, wf *model.Workflow, inputs map[string]any, env map[string]string, reg participant.Registry) (any, error) {
	// Build a CEL environment scoped to this workflow's variable declarations.
	celEnv, err := cel.NewEnv(wf)
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}

	// Initialise execution state with defaults and metadata.
	state := NewState(wf, inputs, env)

	// Execute all top-level flow steps sequentially.
	lastStep, err := runSequential(ctx, wf, wf.Flow, state, celEnv, reg)
	if err != nil {
		state.Execution.Status = "failed"
		return nil, err
	}

	state.Execution.Status = "success"

	// Resolve and return the workflow output expression.
	return resolveOutput(wf.Output, state, celEnv, lastStep)
}
