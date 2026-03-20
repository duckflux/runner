package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/eventhub"
	"github.com/duckflux/runner/internal/model"
	"github.com/duckflux/runner/internal/participant"
)

// withStepTimeout derives a child context that is cancelled after the resolved
// timeout duration. It returns the child context and a cancel function that must
// be called to release resources. If there is no effective timeout, the parent
// context is returned unchanged along with a no-op cancel function.
func withStepTimeout(ctx context.Context, def model.Participant, override *model.ParticipantOverrideStep, wf *model.Workflow) (context.Context, context.CancelFunc) {
	if d := resolveTimeout(def, override, wf); d != nil {
		return context.WithTimeout(ctx, d.Duration)
	}
	return ctx, func() {}
}

// runSequential iterates over a slice of flow steps, executing each in order.
// It propagates a chain value between steps: the output of step N becomes the
// implicit input of step N+1. It returns the final chain value and the name of
// the last participant step that was executed (not skipped).
func runSequential(ctx context.Context, wf *model.Workflow, steps []model.FlowStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (string, any, error) {
	var lastStep string
	for _, step := range steps {
		name, newChain, err := runFlowStep(ctx, wf, step, state, celEnv, reg, hub, chain)
		if err != nil {
			return "", nil, err
		}
		if newChain != nil || name != "" {
			chain = newChain
		}
		if name != "" {
			lastStep = name
		}
	}
	return lastStep, chain, nil
}

// runFlowStep dispatches a single flow step to its appropriate handler and
// returns the participant name that was executed (empty for control-flow steps),
// and the new chain value (output of the step).
func runFlowStep(ctx context.Context, wf *model.Workflow, step model.FlowStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (string, any, error) {
	switch {
	case step.Participant != "":
		out, err := runParticipantStep(ctx, wf, step.Participant, nil, state, celEnv, reg, hub, chain)
		if err != nil {
			return "", nil, err
		}
		return step.Participant, out, nil

	case step.Override != nil:
		name := step.Override.Participant
		out, err := runParticipantStep(ctx, wf, name, step.Override, state, celEnv, reg, hub, chain)
		if err != nil {
			return "", nil, err
		}
		return name, out, nil

	case step.Loop != nil:
		loopChain, err := runLoop(ctx, wf, step.Loop, state, celEnv, reg, hub, chain)
		if err != nil {
			return "", nil, err
		}
		return "", loopChain, nil

	case step.Parallel != nil:
		parallelOut, err := runParallel(ctx, wf, step.Parallel, state, celEnv, reg, hub, chain)
		if err != nil {
			return "", nil, err
		}
		return "", parallelOut, nil

	case step.If != nil:
		return runIf(ctx, wf, step.If, state, celEnv, reg, hub, chain)

	case step.Wait != nil:
		if err := runWait(ctx, wf, step.Wait, state, celEnv, reg, hub); err != nil {
			return "", nil, err
		}
		// wait does not produce output; chain unchanged
		return "", chain, nil

	case step.Set != nil:
		if err := runSet(wf, step.Set, state, celEnv); err != nil {
			return "", nil, err
		}
		// set does not produce output; chain unchanged
		return "", chain, nil

	case step.InlineParticipant != nil:
		def := *step.InlineParticipant
		out, err := runInlineParticipant(ctx, wf, &def, state, celEnv, reg, hub, chain)
		if err != nil {
			return "", nil, err
		}
		name := def.As // may be empty for anonymous
		return name, out, nil

	default:
		return "", nil, fmt.Errorf("unsupported flow step type")
	}
}

// runParticipantStep resolves the participant definition, evaluates its input
// expressions, invokes Execute, and stores the result in state.Steps.
// chain is the implicit I/O chain value from the previous step.
// Returns the step output (for chain propagation) and any error.
func runParticipantStep(ctx context.Context, wf *model.Workflow, name string, override *model.ParticipantOverrideStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (any, error) {
	def, ok := wf.Participants[name]
	if !ok {
		return nil, fmt.Errorf("participant %q not found in workflow definition", name)
	}
	p, ok := reg[name]
	if !ok {
		return nil, fmt.Errorf("participant %q has no registered implementation", name)
	}

	// Inject current timestamp for CEL expressions.
	state.Now = time.Now().UTC()

	// Set CurrentInput to chain so that `when` guard can access it via `input`.
	state.CurrentInput = chain

	// Evaluate the "when" guard, if present. A false result skips this step.
	when := ""
	if override != nil {
		when = override.When
	}
	if when != "" {
		prog, err := celEnv.Compile(when)
		if err != nil {
			return nil, fmt.Errorf("participant %q: compiling when guard: %w", name, err)
		}
		result, err := celEnv.Eval(prog, state)
		if err != nil {
			return nil, fmt.Errorf("participant %q: evaluating when guard: %w", name, err)
		}
		cond, ok := result.(bool)
		if !ok {
			return nil, fmt.Errorf("participant %q: when guard must evaluate to bool, got %T", name, result)
		}
		if !cond {
			state.SetStep(name, &cel.StepResult{Status: "skipped"})
			// Skipped step: chain unchanged (return nil to signal no new chain)
			return nil, nil
		}
	}

	// Determine the effective input: flow-level override takes priority.
	var rawInput interface{}
	if override != nil && override.Input != nil {
		rawInput = override.Input
	} else {
		rawInput = def.Input
	}
	var effectiveCWD string

	// Evaluate input CEL expressions to produce the concrete input value.
	explicitInput, err := evalInput(rawInput, state, celEnv)
	if err != nil {
		return nil, fmt.Errorf("participant %q: evaluating input: %w", name, err)
	}

	// Merge chain input with explicit input per v0.3 spec.
	input, err := mergeChainedInput(chain, explicitInput)
	if err != nil {
		return nil, fmt.Errorf("participant %q: merging chain input: %w", name, err)
	}

	// Set participant-scoped input for CEL expressions during execution.
	state.CurrentInput = input

	// Resolve runtime participant fields for types that support dynamic values.
	execParticipant := p
	switch def.Type {
	case model.ParticipantTypeHTTP:
		resolvedDef, err := resolveHTTPDefinition(def, state, celEnv)
		if err != nil {
			return nil, fmt.Errorf("participant %q: resolving http fields: %w", name, err)
		}
		if hp, ok := p.(*participant.HTTPParticipant); ok {
			execParticipant = hp.WithDefinition(resolvedDef)
		} else {
			execParticipant = participant.NewHTTP(resolvedDef, nil)
		}
	case model.ParticipantTypeExec:
		if ep, ok := p.(*participant.ExecParticipant); ok {
			resolvedDef, err := resolveExecDefinition(ctx, def, wf, state, celEnv)
			if err != nil {
				return nil, fmt.Errorf("participant %q: resolving exec fields: %w", name, err)
			}
			execParticipant = ep.WithDefinition(resolvedDef)
			// capture effective CWD for recording in the step result
			effectiveCWD = resolvedDef.CWD
		}
	case model.ParticipantTypeWorkflow:
		if override != nil && override.Workflow != "" {
			if wp, ok := p.(*participant.WorkflowParticipant); ok {
				execParticipant = wp.WithPath(override.Workflow)
			}
		}
	case model.ParticipantTypeEmit:
		resolvedDef, err := resolveEmitDefinition(def, state, celEnv)
		if err != nil {
			return nil, fmt.Errorf("participant %q: resolving emit payload: %w", name, err)
		}
		// Always create a new EmitParticipant with the current engine hub, so
		// that emit steps work correctly even when BuildRegistry received a nil hub.
		execParticipant = participant.NewEmit(resolvedDef, hub)
	}

	// Apply timeout: derive a child context with the resolved deadline, if any.
	stepCtx, cancel := withStepTimeout(ctx, def, override, wf)
	defer cancel()

	// Determine the onError strategy up-front so we can pass a retry config to
	// executeWithRetry when the strategy is "retry".
	onErr := resolveOnError(def, override, wf)
	var retryConfig *model.RetryConfig
	if onErr == "retry" {
		retryConfig = resolveRetry(def, override)
	}

	// Execute the participant, retrying with exponential backoff when configured.
	startedAt := time.Now()
	slog.Debug("step starting", "participant", name)
	out, retries, execErr := executeWithRetry(stepCtx, func() (any, error) {
		return execParticipant.Execute(stepCtx, input)
	}, retryConfig)
	finishedAt := time.Now()
	elapsed := finishedAt.Sub(startedAt)
	startedAtStr := startedAt.UTC().Format(time.RFC3339)
	finishedAtStr := finishedAt.UTC().Format(time.RFC3339)
	durationStr := elapsed.String()

	if execErr != nil {
		slog.Debug("step failed", "participant", name, "duration", durationStr, "error", execErr)
		switch onErr {
		case "skip":
			state.SetStep(name, &cel.StepResult{
				Status:     "skipped",
				StartedAt:  startedAtStr,
				FinishedAt: finishedAtStr,
				Duration:   durationStr,
				Error:      execErr.Error(),
			})
			// Skipped on error: chain unchanged (return nil to signal no new chain)
			return nil, nil
		case "retry":
			state.SetStep(name, &cel.StepResult{
				Status:     "failed",
				Retries:    int64(retries),
				StartedAt:  startedAtStr,
				FinishedAt: finishedAtStr,
				Duration:   durationStr,
				Error:      execErr.Error(),
				CWD:        effectiveCWD,
			})
			return nil, fmt.Errorf("participant %q failed after %d retries: %w", name, retries, execErr)
		case "fail":
			state.SetStep(name, &cel.StepResult{
				Status:     "failed",
				StartedAt:  startedAtStr,
				FinishedAt: finishedAtStr,
				Duration:   durationStr,
				Error:      execErr.Error(),
				CWD:        effectiveCWD,
			})
			return nil, fmt.Errorf("participant %q failed: %w", name, execErr)
		default:
			// onErr is a participant name — execute it as a fallback (redirect).
			// Force onError="fail" for the fallback to prevent infinite redirect chains.
			state.SetStep(name, &cel.StepResult{
				Status:     "failed",
				StartedAt:  startedAtStr,
				FinishedAt: finishedAtStr,
				Duration:   durationStr,
				Error:      execErr.Error(),
				CWD:        effectiveCWD,
			})
			fallbackOverride := &model.ParticipantOverrideStep{OnError: "fail"}
			if _, redirectErr := runParticipantStep(ctx, wf, onErr, fallbackOverride, state, celEnv, reg, hub, chain); redirectErr != nil {
				return nil, fmt.Errorf("participant %q failed and fallback %q also failed: %w", name, onErr, redirectErr)
			}
			return nil, nil
		}
	}

	// Apply JSON auto-detection: attempt to parse string outputs as JSON.
	out = autoDetectJSON(out)

	// Set participant-scoped output for CEL expressions.
	state.CurrentOutput = out

	slog.Debug("step completed", "participant", name, "status", "success", "duration", durationStr)
	state.SetStep(name, &cel.StepResult{
		Output:     out,
		Status:     "success",
		Retries:    int64(retries),
		StartedAt:  startedAtStr,
		FinishedAt: finishedAtStr,
		Duration:   durationStr,
		CWD:        effectiveCWD,
	})
	return out, nil
}

// runInlineParticipant executes an inline participant definition. If `As` is
// set, the step result is stored under that name. For anonymous inline
// participants (no `As`), the step runs and contributes to the chain but does
// not create a named binding.
func runInlineParticipant(ctx context.Context, wf *model.Workflow, def *model.Participant, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (any, error) {
	if def.As != "" {
		// Named inline: delegate to runParticipantStep via the synthetic name.
		override := &model.ParticipantOverrideStep{
			When: def.When,
		}
		return runParticipantStep(ctx, wf, def.As, override, state, celEnv, reg, hub, chain)
	}

	// Anonymous inline: build a one-off participant and execute directly.
	p, err := participant.BuildOne(*def, state.Env, nil, hub)
	if err != nil {
		return nil, fmt.Errorf("building anonymous inline participant: %w", err)
	}

	state.Now = time.Now().UTC()
	state.CurrentInput = chain

	// Evaluate when guard if present.
	if def.When != "" {
		prog, err := celEnv.Compile(def.When)
		if err != nil {
			return nil, fmt.Errorf("anonymous inline: compiling when guard: %w", err)
		}
		result, err := celEnv.Eval(prog, state)
		if err != nil {
			return nil, fmt.Errorf("anonymous inline: evaluating when guard: %w", err)
		}
		cond, ok := result.(bool)
		if !ok {
			return nil, fmt.Errorf("anonymous inline: when guard must evaluate to bool, got %T", result)
		}
		if !cond {
			return nil, nil // skipped: chain unchanged
		}
	}

	// Evaluate explicit input and merge with chain.
	explicitInput, err := evalInput(def.Input, state, celEnv)
	if err != nil {
		return nil, fmt.Errorf("anonymous inline: evaluating input: %w", err)
	}
	input, err := mergeChainedInput(chain, explicitInput)
	if err != nil {
		return nil, fmt.Errorf("anonymous inline: merging chain input: %w", err)
	}
	state.CurrentInput = input

	// Resolve exec CWD if needed.
	if def.Type == model.ParticipantTypeExec && def.CWD != "" {
		resolvedDef, err := resolveExecDefinition(ctx, *def, wf, state, celEnv)
		if err != nil {
			return nil, fmt.Errorf("anonymous inline: resolving exec fields: %w", err)
		}
		p = participant.NewExec(resolvedDef, state.Env)
	}

	out, err := p.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("anonymous inline: execution failed: %w", err)
	}

	out = autoDetectJSON(out)
	state.CurrentOutput = out
	return out, nil
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

func resolveRetry(def model.Participant, override *model.ParticipantOverrideStep) *model.RetryConfig {
	if override != nil && override.Retry != nil {
		return override.Retry
	}
	return def.Retry
}

// resolveExecDefinition resolves the effective cwd for an exec participant
// using precedence: participant.cwd > defaults.cwd > CLI --cwd > process cwd.
// participant/defaults cwd values may be CEL expressions; non-CEL strings are
// treated as literals.
func resolveExecDefinition(ctx context.Context, def model.Participant, wf *model.Workflow, state *cel.State, env *cel.Environment) (model.Participant, error) {
	base := baseCWDFromContext(ctx)
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return model.Participant{}, fmt.Errorf("resolving current working directory: %w", err)
		}
		base = wd
	}

	resolved := def

	// 1) participant.cwd
	if def.CWD != "" {
		cwd, err := evalMaybeCELString(def.CWD, state, env)
		if err != nil {
			return model.Participant{}, fmt.Errorf("participant.cwd: %w", err)
		}
		if cwd != "" {
			resolved.CWD = toAbsoluteCWD(base, cwd)
			return resolved, nil
		}
	}

	// 2) defaults.cwd
	if wf.Defaults != nil && wf.Defaults.CWD != "" {
		cwd, err := evalMaybeCELString(wf.Defaults.CWD, state, env)
		if err != nil {
			return model.Participant{}, fmt.Errorf("defaults.cwd: %w", err)
		}
		if cwd != "" {
			resolved.CWD = toAbsoluteCWD(base, cwd)
			return resolved, nil
		}
	}

	// 3) CLI --cwd (stored in context), 4) process cwd (already captured in base)
	resolved.CWD = base
	return resolved, nil
}

func resolveEmitDefinition(def model.Participant, state *cel.State, env *cel.Environment) (model.Participant, error) {
	resolved := def
	if def.Payload != nil {
		payload, err := evalMaybeCEL(def.Payload, state, env)
		if err != nil {
			return model.Participant{}, err
		}
		resolved.Payload = payload
	}
	return resolved, nil
}

func toAbsoluteCWD(base string, cwd string) string {
	if filepath.IsAbs(cwd) {
		return cwd
	}
	return filepath.Join(base, cwd)
}

// resolveHTTPDefinition evaluates HTTP participant definition fields against
// the current runtime state. Strings that are not valid CEL expressions are
// preserved as literals.
func resolveHTTPDefinition(def model.Participant, state *cel.State, env *cel.Environment) (model.Participant, error) {
	resolved := def

	url, err := evalMaybeCELString(def.URL, state, env)
	if err != nil {
		return model.Participant{}, fmt.Errorf("url: %w", err)
	}
	method, err := evalMaybeCELString(def.Method, state, env)
	if err != nil {
		return model.Participant{}, fmt.Errorf("method: %w", err)
	}

	resolved.URL = url
	resolved.Method = method

	if def.Headers != nil {
		headers := make(map[string]string, len(def.Headers))
		for k, v := range def.Headers {
			evalV, err := evalMaybeCELString(v, state, env)
			if err != nil {
				return model.Participant{}, fmt.Errorf("headers.%s: %w", k, err)
			}
			headers[k] = evalV
		}
		resolved.Headers = headers
	}

	if def.Body != nil {
		body, err := evalMaybeCEL(def.Body, state, env)
		if err != nil {
			return model.Participant{}, fmt.Errorf("body: %w", err)
		}
		resolved.Body = body
	}

	return resolved, nil
}

// evalMaybeCELString evaluates raw as CEL when possible; if raw does not
// compile as CEL it is returned unchanged as a literal string.
func evalMaybeCELString(raw string, state *cel.State, env *cel.Environment) (string, error) {
	if raw == "" {
		return "", nil
	}

	prog, err := env.Compile(raw)
	if err != nil {
		return raw, nil
	}

	out, err := env.Eval(prog, state)
	if err != nil {
		return "", err
	}

	if out == nil {
		return "", nil
	}
	return fmt.Sprint(out), nil
}

// evalMaybeCEL recursively evaluates CEL expressions where possible; invalid
// CEL strings are preserved as literals so static HTTP config remains valid.
func evalMaybeCEL(raw interface{}, state *cel.State, env *cel.Environment) (any, error) {
	switch v := raw.(type) {
	case string:
		prog, err := env.Compile(v)
		if err != nil {
			return v, nil
		}
		return env.Eval(prog, state)
	case map[string]interface{}:
		result := make(map[string]any, len(v))
		for key, val := range v {
			evaluated, err := evalMaybeCEL(val, state, env)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", key, err)
			}
			result[key] = evaluated
		}
		return result, nil
	case []interface{}:
		result := make([]any, len(v))
		for i, item := range v {
			evaluated, err := evalMaybeCEL(item, state, env)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			result[i] = evaluated
		}
		return result, nil
	default:
		return raw, nil
	}
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
//  3. No explicit output           → return the final chain value (v0.3), falling back to last step output.
func resolveOutput(output *model.WorkflowOutput, state *cel.State, env *cel.Environment, lastStep string, finalChain any) (any, error) {
	if output == nil {
		if finalChain != nil {
			return finalChain, nil
		}
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
		result, err := evalOutputMap(output.Map, state, env)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	if output.MapField != nil {
		result, err := evalOutputMap(output.MapField, state, env)
		if err != nil {
			return nil, err
		}
		if len(output.Schema) > 0 {
			if err := validateOutputSchema(output.Schema, result); err != nil {
				return nil, err
			}
		}
		return result, nil
	}

	return stepOutput(state, lastStep), nil
}

func evalOutputMap(m map[string]string, state *cel.State, env *cel.Environment) (map[string]any, error) {
	result := make(map[string]any, len(m))
	for key, expr := range m {
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

func validateOutputSchema(schema map[string]model.InputField, values map[string]any) error {
	for key, field := range schema {
		val, ok := values[key]
		if field.Required && !ok {
			return fmt.Errorf("output schema: required field %q is missing", key)
		}
		if !ok {
			continue
		}
		if err := validateValueAgainstField(key, val, field); err != nil {
			return err
		}
	}
	return nil
}

func validateValueAgainstField(key string, value any, field model.InputField) error {
	expected := field.Type
	if expected == "" {
		expected = "string"
	}
	switch expected {
	case "string":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("output schema: field %q must be string, got %T", key, value)
		}
		if field.MinLength != nil && len(s) < *field.MinLength {
			return fmt.Errorf("output schema: field %q length must be >= %d", key, *field.MinLength)
		}
		if field.MaxLength != nil && len(s) > *field.MaxLength {
			return fmt.Errorf("output schema: field %q length must be <= %d", key, *field.MaxLength)
		}
		if field.Pattern != "" {
			re, err := regexp.Compile(field.Pattern)
			if err != nil {
				return fmt.Errorf("output schema: invalid pattern for field %q: %w", key, err)
			}
			if !re.MatchString(s) {
				return fmt.Errorf("output schema: field %q does not match pattern %q", key, field.Pattern)
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("output schema: field %q must be boolean, got %T", key, value)
		}
	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("output schema: field %q must be integer, got %T", key, value)
		}
		if n, ok := toFloat(value); ok {
			if field.Minimum != nil && n < *field.Minimum {
				return fmt.Errorf("output schema: field %q must be >= %v", key, *field.Minimum)
			}
			if field.Maximum != nil && n > *field.Maximum {
				return fmt.Errorf("output schema: field %q must be <= %v", key, *field.Maximum)
			}
		}
	case "number":
		n, ok := toFloat(value)
		if !ok {
			return fmt.Errorf("output schema: field %q must be number, got %T", key, value)
		}
		if field.Minimum != nil && n < *field.Minimum {
			return fmt.Errorf("output schema: field %q must be >= %v", key, *field.Minimum)
		}
		if field.Maximum != nil && n > *field.Maximum {
			return fmt.Errorf("output schema: field %q must be <= %v", key, *field.Maximum)
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("output schema: field %q must be array, got %T", key, value)
		}
		if field.Items != nil {
			for i, item := range arr {
				if err := validateValueAgainstField(fmt.Sprintf("%s[%d]", key, i), item, *field.Items); err != nil {
					return err
				}
			}
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("output schema: field %q must be object, got %T", key, value)
		}
	}

	if len(field.Enum) > 0 {
		found := false
		for _, v := range field.Enum {
			if reflect.DeepEqual(v, value) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("output schema: field %q value %v is not in enum", key, value)
		}
	}

	return nil
}

func isInteger(v any) bool {
	switch n := v.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return math.Trunc(n) == n
	case float32:
		return float32(math.Trunc(float64(n))) == n
	default:
		return false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
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
