package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
	"github.com/duckflux/runner/internal/participant"
)

// runSequential iterates over a slice of flow steps, executing each in order.
// It returns the name of the last participant step that was executed (not skipped),
// which is used to derive the implicit workflow output when no explicit output is
// defined. An error from any step aborts execution immediately.
func runSequential(ctx context.Context, wf *model.Workflow, steps []model.FlowStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) (string, error) {
	var lastStep string
	for _, step := range steps {
		name, err := runFlowStep(ctx, wf, step, state, celEnv, reg)
		if err != nil {
			return "", err
		}
		if name != "" {
			lastStep = name
		}
	}
	return lastStep, nil
}

// runFlowStep dispatches a single flow step to its appropriate handler and
// returns the participant name that was executed (empty for control-flow steps).
func runFlowStep(ctx context.Context, wf *model.Workflow, step model.FlowStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) (string, error) {
	switch {
	case step.Participant != "":
		if err := runParticipantStep(ctx, wf, step.Participant, nil, state, celEnv, reg); err != nil {
			return "", err
		}
		return step.Participant, nil

	case step.Override != nil:
		name := step.Override.Participant
		if err := runParticipantStep(ctx, wf, name, step.Override, state, celEnv, reg); err != nil {
			return "", err
		}
		return name, nil

	case step.Loop != nil:
		if err := runLoop(ctx, wf, step.Loop, state, celEnv, reg); err != nil {
			return "", err
		}
		return "", nil

	case step.Parallel != nil:
		if err := runParallel(ctx, wf, step.Parallel, state, celEnv, reg); err != nil {
			return "", err
		}
		return "", nil

	case step.If != nil:
		return runIf(ctx, wf, step.If, state, celEnv, reg)

	default:
		return "", fmt.Errorf("unsupported flow step type")
	}
}

// runParticipantStep resolves the participant definition, evaluates its input
// expressions, invokes Execute, and stores the result in state.Steps.
func runParticipantStep(ctx context.Context, wf *model.Workflow, name string, override *model.ParticipantOverrideStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry) error {
	def, ok := wf.Participants[name]
	if !ok {
		return fmt.Errorf("participant %q not found in workflow definition", name)
	}
	p, ok := reg[name]
	if !ok {
		return fmt.Errorf("participant %q has no registered implementation", name)
	}

	// Evaluate the "when" guard, if present. A false result skips this step.
	when := ""
	if override != nil {
		when = override.When
	}
	if when != "" {
		prog, err := celEnv.Compile(when)
		if err != nil {
			return fmt.Errorf("participant %q: compiling when guard: %w", name, err)
		}
		result, err := celEnv.Eval(prog, state)
		if err != nil {
			return fmt.Errorf("participant %q: evaluating when guard: %w", name, err)
		}
		if cond, _ := result.(bool); !cond {
			state.SetStep(name, &cel.StepResult{Status: "skipped"})
			return nil
		}
	}

	// Determine the effective input: flow-level override takes priority.
	var rawInput interface{}
	if override != nil && override.Input != nil {
		rawInput = override.Input
	} else {
		rawInput = def.Input
	}

	// Evaluate input CEL expressions to produce the concrete input value.
	input, err := evalInput(rawInput, state, celEnv)
	if err != nil {
		return fmt.Errorf("participant %q: evaluating input: %w", name, err)
	}

	// Execute the participant.
	out, execErr := p.Execute(ctx, input)
	if execErr != nil {
		onErr := resolveOnError(def, override, wf)
		switch onErr {
		case "skip":
			state.SetStep(name, &cel.StepResult{Status: "skipped"})
			return nil
		default:
			// "fail" is the default; redirect (to another participant) is Phase 4d.
			state.SetStep(name, &cel.StepResult{Status: "failed"})
			return fmt.Errorf("participant %q failed: %w", name, execErr)
		}
	}

	// Apply JSON auto-detection: attempt to parse string outputs as JSON.
	out = autoDetectJSON(out)

	state.SetStep(name, &cel.StepResult{
		Output:  out,
		Status:  "success",
		Retries: 0,
	})
	return nil
}

// resolveOnError returns the effective onError strategy for a step invocation.
// Precedence: flow override > participant definition > workflow defaults > "fail".
func resolveOnError(def model.Participant, override *model.ParticipantOverrideStep, wf *model.Workflow) string {
	if override != nil && override.OnError != "" {
		return override.OnError
	}
	if def.OnError != "" {
		return def.OnError
	}
	if wf.Defaults != nil && wf.Defaults.OnError != "" {
		return wf.Defaults.OnError
	}
	return "fail"
}

// evalInput evaluates a participant input definition against the current state.
// String values are treated as CEL expressions; maps recurse into each value;
// other scalar types (int, float64, bool, etc.) are passed through unchanged.
func evalInput(raw interface{}, state *cel.State, env *cel.Environment) (any, error) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case string:
		prog, err := env.Compile(v)
		if err != nil {
			return nil, fmt.Errorf("compiling CEL expression %q: %w", v, err)
		}
		return env.Eval(prog, state)
	case map[string]interface{}:
		result := make(map[string]any, len(v))
		for key, val := range v {
			evaluated, err := evalInput(val, state, env)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", key, err)
			}
			result[key] = evaluated
		}
		return result, nil
	default:
		// Scalars (int, float64, bool, etc.) are passed through as-is.
		return raw, nil
	}
}

// autoDetectJSON attempts to unmarshal a string value as JSON. If the string
// starts with '{' or '[' and unmarshals successfully, the parsed Go value is
// returned. Otherwise the original value is returned unchanged.
func autoDetectJSON(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	s = strings.TrimSpace(s)
	if len(s) == 0 || (s[0] != '{' && s[0] != '[') {
		return v
	}
	var parsed any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return v
	}
	return parsed
}

// resolveOutput evaluates the workflow output definition against the final state.
//
// Priority:
//  1. Explicit output expression   → evaluate CEL, return result.
//  2. Explicit output map          → evaluate each CEL expression, return map.
//  3. No explicit output           → return the last executed step's output.
func resolveOutput(output *model.WorkflowOutput, state *cel.State, env *cel.Environment, lastStep string) (any, error) {
	if output == nil {
		return stepOutput(state, lastStep), nil
	}

	if output.Expression != "" {
		prog, err := env.Compile(output.Expression)
		if err != nil {
			return nil, fmt.Errorf("compiling output expression: %w", err)
		}
		return env.Eval(prog, state)
	}

	if output.Map != nil {
		result := make(map[string]any, len(output.Map))
		for key, expr := range output.Map {
			prog, err := env.Compile(expr)
			if err != nil {
				return nil, fmt.Errorf("compiling output field %q: %w", key, err)
			}
			val, err := env.Eval(prog, state)
			if err != nil {
				return nil, fmt.Errorf("evaluating output field %q: %w", key, err)
			}
			result[key] = val
		}
		return result, nil
	}

	return stepOutput(state, lastStep), nil
}

// stepOutput returns the output value recorded for the named step, or nil if
// the step has not run or produced no output.
func stepOutput(state *cel.State, name string) any {
	if name == "" {
		return nil
	}
	if result, ok := state.Steps[name]; ok {
		return result.Output
	}
	return nil
}
