package participant

import (
	"context"
	"encoding/json"
	"fmt"
)

// SubWorkflowRunnerFunc is the signature for a function that can parse and
// execute a sub-workflow file end-to-end. The implementation is provided by
// the wiring layer (e.g. the CLI) so that the participant package does not need
// to import the engine package and create a circular dependency.
//
// path is the file-system path to the sub-workflow YAML file.
// inputs are the evaluated key/value pairs mapped from the parent step input.
// env contains the inherited environment variable map (env.*).
//
// The function returns the child workflow's output or an error.
type SubWorkflowRunnerFunc func(ctx context.Context, path string, inputs map[string]any, env map[string]string) (any, error)

// WorkflowParticipant executes a sub-workflow defined in a separate YAML file.
// It passes the evaluated step input as the child workflow's inputs, inherits
// the parent's environment variables, and returns the child's output as its
// own Execute result.
type WorkflowParticipant struct {
	path     string
	env      map[string]string
	runnerFn SubWorkflowRunnerFunc
}

// NewWorkflow constructs a WorkflowParticipant. path is the file-system path to
// the referenced sub-workflow YAML file. env is the parent workflow's
// environment variable map, inherited by the child execution. runnerFn is
// called by Execute to parse and run the sub-workflow; it must not be nil.
func NewWorkflow(path string, env map[string]string, runnerFn SubWorkflowRunnerFunc) (*WorkflowParticipant, error) {
	if path == "" {
		return nil, fmt.Errorf("workflow participant: path must not be empty")
	}
	if runnerFn == nil {
		return nil, fmt.Errorf("workflow participant: runnerFn must not be nil")
	}
	return &WorkflowParticipant{
		path:     path,
		env:      env,
		runnerFn: runnerFn,
	}, nil
}

// WithPath returns a copy of the participant configured with a different
// workflow path, preserving env and runner function.
func (w *WorkflowParticipant) WithPath(path string) *WorkflowParticipant {
	return &WorkflowParticipant{
		path:     path,
		env:      w.env,
		runnerFn: w.runnerFn,
	}
}

// Execute runs the referenced sub-workflow. input is the evaluated step input
// (typically a map[string]any produced by CEL evaluation). nil input results
// in an empty inputs map being passed to the child workflow. The child's
// output is returned directly as this step's output.
func (w *WorkflowParticipant) Execute(ctx context.Context, input any) (any, error) {
	inputs, err := inputToWorkflowMap(input)
	if err != nil {
		return nil, fmt.Errorf("workflow participant %q: converting input: %w", w.path, err)
	}

	out, err := w.runnerFn(ctx, w.path, inputs, w.env)
	if err != nil {
		return nil, fmt.Errorf("workflow participant %q: %w", w.path, err)
	}
	return out, nil
}

// inputToWorkflowMap converts an Execute input value to a map[string]any
// suitable for use as a child workflow's inputs.
//
//   - nil            → empty map
//   - map[string]any → returned as-is
//   - string         → JSON-unmarshalled when it looks like a JSON object;
//     otherwise wrapped as {"value": s}
//   - other          → JSON round-tripped into map[string]any; scalars that
//     cannot be represented as a map are wrapped as {"value": v}
func inputToWorkflowMap(input any) (map[string]any, error) {
	if input == nil {
		return map[string]any{}, nil
	}

	if m, ok := input.(map[string]any); ok {
		return m, nil
	}

	if s, ok := input.(string); ok {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			return m, nil
		}
		return map[string]any{"value": s}, nil
	}

	// For all other types, JSON round-trip to obtain a map.
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshalling input to JSON: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		// The value is a scalar (number, bool, …); wrap it.
		return map[string]any{"value": input}, nil
	}
	return m, nil
}
