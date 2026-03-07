package cel

import (
	"sync"

	"github.com/duckflux/runner/internal/model"
	gcel "github.com/google/cel-go/cel"
)

// Environment wraps a cel.Env configured for a specific workflow, together with
// the list of participant names that were declared as variables.
type Environment struct {
	env          *gcel.Env
	participants []string
	mu           sync.RWMutex
	programs     map[string]gcel.Program
}

// NewEnv builds a CEL environment for the given workflow, declaring all runtime
// variable types so that expressions can be type-checked at parse/lint time.
func NewEnv(wf *model.Workflow) (*Environment, error) {
	opts := []gcel.EnvOption{
		gcel.Variable("workflow", gcel.MapType(gcel.StringType, gcel.DynType)),
		gcel.Variable("execution", gcel.MapType(gcel.StringType, gcel.DynType)),
		gcel.Variable("input", gcel.MapType(gcel.StringType, gcel.DynType)),
		gcel.Variable("env", gcel.MapType(gcel.StringType, gcel.StringType)),
		// "loop" is a CEL reserved identifier, so the loop context variable is
		// declared internally as "_loop". Expressions written with "loop." are
		// transparently rewritten to "_loop." by Compile before type-checking.
		gcel.Variable("_loop", gcel.MapType(gcel.StringType, gcel.DynType)),
	}

	// Declare each participant name as a map variable for step-result access.
	participants := make([]string, 0, len(wf.Participants))
	for name := range wf.Participants {
		opts = append(opts, gcel.Variable(name, gcel.MapType(gcel.StringType, gcel.DynType)))
		participants = append(participants, name)
	}

	env, err := gcel.NewEnv(opts...)
	if err != nil {
		return nil, err
	}
	return &Environment{
		env:          env,
		participants: participants,
		programs:     make(map[string]gcel.Program),
	}, nil
}

// Bindings converts a State into a flat map suitable for passing to cel.Program.Eval.
// Participants that have not yet produced a result are included with an empty map so
// that declared variables are always present in the activation.
func (e *Environment) Bindings(s *State) map[string]any {
	input := s.Input
	if input == nil {
		input = map[string]any{}
	}
	env := s.Env
	if env == nil {
		env = map[string]string{}
	}

	vars := map[string]any{
		"workflow": map[string]any{
			"id":      s.Workflow.ID,
			"name":    s.Workflow.Name,
			"version": s.Workflow.Version,
		},
		"execution": map[string]any{
			"id":        s.Execution.ID,
			"number":    s.Execution.Number,
			"startedAt": s.Execution.StartedAt,
			"status":    s.Execution.Status,
			"context":   s.Execution.Context,
		},
		"input": input,
		"env":   env,
		// "_loop" is used because "loop" is a reserved identifier in CEL.
		"_loop": map[string]any{},
	}

	if s.Loop != nil {
		vars["_loop"] = map[string]any{
			"index":     s.Loop.Index,
			"iteration": s.Loop.Iteration,
			"first":     s.Loop.First,
			"last":      s.Loop.Last,
		}
	}

	// Provide bindings for all declared participants; default to empty map for unrun steps.
	// Take a read lock so that parallel goroutines writing to Steps don't race.
	s.mu.RLock()
	for _, name := range e.participants {
		if result, ok := s.Steps[name]; ok {
			vars[name] = map[string]any{
				"output":     result.Output,
				"status":     result.Status,
				"retries":    result.Retries,
				"startedAt":  result.StartedAt,
				"finishedAt": result.FinishedAt,
				"duration":   result.Duration,
				"error":      result.Error,
			}
		} else {
			vars[name] = map[string]any{}
		}
	}
	s.mu.RUnlock()

	return vars
}
