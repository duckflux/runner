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
	wfForRuntime := workflowWithInlineParticipants(wf)

	// Build a CEL environment scoped to this workflow's variable declarations.
	celEnv, err := cel.NewEnv(wfForRuntime)
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}
	if err := celEnv.PrecompileWorkflow(wfForRuntime); err != nil {
		return nil, fmt.Errorf("precompiling CEL expressions: %w", err)
	}

	// Initialise execution state with defaults and metadata.
	state := NewState(wf, inputs, env)
	if base := baseCWDFromContext(ctx); base != "" {
		state.Execution.CWD = base
		if state.Execution.Context == nil {
			state.Execution.Context = map[string]any{}
		}
		state.Execution.Context["cwd"] = base
	}

	// Execute all top-level flow steps sequentially with nil initial chain.
	lastStep, finalChain, err := runSequential(ctx, wf, wf.Flow, state, celEnv, reg, nil)
	if err != nil {
		state.Execution.Status = "failed"
		return nil, err
	}

	state.Execution.Status = "success"

	// Resolve and return the workflow output expression.
	// If no explicit output is defined, return the final chain value.
	return resolveOutput(wf.Output, state, celEnv, lastStep, finalChain)
}

func workflowWithInlineParticipants(wf *model.Workflow) *model.Workflow {
	synthetic := make(map[string]model.Participant, len(wf.Participants))
	for k, v := range wf.Participants {
		synthetic[k] = v
	}
	var walk func([]model.FlowStep)
	walk = func(steps []model.FlowStep) {
		for _, s := range steps {
			if s.InlineParticipant != nil && s.InlineParticipant.As != "" {
				synthetic[s.InlineParticipant.As] = *s.InlineParticipant
			}
			if s.Loop != nil {
				walk(s.Loop.Steps)
			}
			if s.Parallel != nil {
				walk(s.Parallel.Steps)
			}
			if s.If != nil {
				walk(s.If.Then)
				walk(s.If.Else)
			}
		}
	}
	walk(wf.Flow)
	wfCopy := *wf
	wfCopy.Participants = synthetic
	return &wfCopy
}
