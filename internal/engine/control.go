package engine

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/eventhub"
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
func runLoop(ctx context.Context, wf *model.Workflow, step *model.LoopStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (any, error) {
	// Pre-compile the until expression once if provided.
	var hasUntil bool
	untilExpr := step.Until
	if untilExpr != "" {
		hasUntil = true
		// If the loop uses a custom `as` name, rewrite occurrences of that
		// name to the internal "loop" identifier so CEL can evaluate it.
		if step.As != "" {
			untilExpr = strings.ReplaceAll(untilExpr, step.As+".", "loop.")
		}
	}
	maxIterations, hasMax, err := resolveLoopMax(step.Max, state, celEnv, step.As)
	if err != nil {
		return nil, err
	}

	// Safety guard: without either until or max the loop would be infinite.
	if !hasUntil && !hasMax {
		return nil, fmt.Errorf("loop: neither until nor max specified; aborting to prevent infinite loop")
	}

	// Save and restore the outer loop context to support nested loops.
	prevLoop := state.Loop
	defer func() { state.Loop = prevLoop }()

	var iteration int64
	for {
		iteration++
		index := iteration - 1

		isLast := hasMax && int(iteration) == maxIterations

		state.Loop = &cel.LoopContext{
			Index:     index,
			Iteration: iteration,
			First:     iteration == 1,
			Last:      isLast,
		}

		body := step.Steps
		if step.As != "" {
			body = rewriteFlowStepsForLoopAs(step.Steps, step.As)
		}
		// Chain entering iteration N is the result of iteration N-1.
		_, newChain, err := runSequential(ctx, wf, body, state, celEnv, reg, hub, chain)
		if err != nil {
			return nil, err
		}
		chain = newChain

		// Evaluate the until condition after the body has run.
		if hasUntil {
			prog, err := celEnv.Compile(untilExpr)
			if err != nil {
				return nil, fmt.Errorf("loop: compiling until expression: %w", err)
			}
			state.Now = time.Now().UTC()
			result, err := celEnv.Eval(prog, state)
			if err != nil {
				return nil, fmt.Errorf("loop: evaluating until expression: %w", err)
			}
			done, ok := result.(bool)
			if !ok {
				return nil, fmt.Errorf("loop: until expression must evaluate to bool, got %T", result)
			}
			if done {
				break
			}
		}

		if hasMax && int(iteration) >= maxIterations {
			break
		}
	}
	// Chain after loop is result of last step in last iteration.
	return chain, nil
}

func resolveLoopMax(raw interface{}, state *cel.State, celEnv *cel.Environment, loopAlias string) (int, bool, error) {
	if raw == nil {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case int:
		if v <= 0 {
			return 0, false, fmt.Errorf("loop: max must be greater than zero")
		}
		return v, true, nil
	case int64:
		if v <= 0 {
			return 0, false, fmt.Errorf("loop: max must be greater than zero")
		}
		return int(v), true, nil
	case float64:
		if v <= 0 || math.Trunc(v) != v {
			return 0, false, fmt.Errorf("loop: max must be a positive integer")
		}
		return int(v), true, nil
	case string:
		expr := v
		if loopAlias != "" {
			expr = strings.ReplaceAll(expr, loopAlias+".", "loop.")
		}
		prog, err := celEnv.Compile(expr)
		if err != nil {
			return 0, false, fmt.Errorf("loop: compiling max expression: %w", err)
		}
		res, err := celEnv.Eval(prog, state)
		if err != nil {
			return 0, false, fmt.Errorf("loop: evaluating max expression: %w", err)
		}
		switch n := res.(type) {
		case int64:
			if n <= 0 {
				return 0, false, fmt.Errorf("loop: max expression must evaluate to a positive integer")
			}
			return int(n), true, nil
		case int:
			if n <= 0 {
				return 0, false, fmt.Errorf("loop: max expression must evaluate to a positive integer")
			}
			return n, true, nil
		case float64:
			if n <= 0 || math.Trunc(n) != n {
				return 0, false, fmt.Errorf("loop: max expression must evaluate to a positive integer")
			}
			return int(n), true, nil
		default:
			return 0, false, fmt.Errorf("loop: max expression must evaluate to a positive integer, got %T", res)
		}
	default:
		return 0, false, fmt.Errorf("loop: max must be an integer or CEL expression")
	}
}

// runParallel executes all branches of a parallel step concurrently using one
// goroutine per participant. A derived cancellable context is shared across all
// goroutines: if any branch fails, cancel is called so that still-running
// branches are signalled to stop. The first error encountered is returned.
// Writes to state.Steps are made thread-safely via state.SetStep.
func runParallel(ctx context.Context, wf *model.Workflow, step *model.ParallelStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (any, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make([]error, len(step.Steps))
	outputs := make([]any, len(step.Steps))
	var wg sync.WaitGroup

	for i, branch := range step.Steps {
		wg.Add(1)
		go func(idx int, b model.FlowStep) {
			defer wg.Done()
			// Each branch starts with the same incoming chain.
			_, branchChain, err := runSequential(ctx, wf, []model.FlowStep{b}, state, celEnv, reg, hub, chain)
			if err != nil {
				errs[idx] = err
				cancel()
			} else {
				outputs[idx] = branchChain
			}
		}(i, branch)
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	// Chain after parallel is an ordered array of branch outputs.
	return outputs, nil
}

// runIf evaluates the CEL condition and executes either the then or the else
// branch. It returns the name of the last participant executed (if any), which
// may propagate up as the implicit workflow output.
func runIf(ctx context.Context, wf *model.Workflow, step *model.IfStep, state *cel.State, celEnv *cel.Environment, reg participant.Registry, hub *eventhub.Hub, chain any) (string, any, error) {
	prog, err := celEnv.Compile(step.Condition)
	if err != nil {
		return "", nil, fmt.Errorf("if: compiling condition: %w", err)
	}
	state.Now = time.Now().UTC()
	result, err := celEnv.Eval(prog, state)
	if err != nil {
		return "", nil, fmt.Errorf("if: evaluating condition: %w", err)
	}

	if cond, ok := result.(bool); ok && cond {
		name, branchChain, err := runSequential(ctx, wf, step.Then, state, celEnv, reg, hub, chain)
		return name, branchChain, err
	} else if !ok {
		return "", nil, fmt.Errorf("if: condition must evaluate to bool, got %T", result)
	}
	if len(step.Else) > 0 {
		name, branchChain, err := runSequential(ctx, wf, step.Else, state, celEnv, reg, hub, chain)
		return name, branchChain, err
	}
	// False branch without else: chain unchanged.
	return "", chain, nil
}

// runSet evaluates each CEL expression in the set step and writes the result
// into execution.context. It does not produce output and the I/O chain passes
// through unchanged.
func runSet(wf *model.Workflow, step *model.SetStep, state *cel.State, celEnv *cel.Environment) error {
	state.Now = time.Now().UTC()
	for key, expr := range step.Values {
		prog, err := celEnv.Compile(expr)
		if err != nil {
			return fmt.Errorf("set %q: compiling expression: %w", key, err)
		}
		val, err := celEnv.Eval(prog, state)
		if err != nil {
			return fmt.Errorf("set %q: evaluating expression: %w", key, err)
		}
		state.Execution.Context[key] = val
	}
	return nil
}

// rewriteFlowStepsForLoopAs returns a new slice of FlowStep where occurrences
// of the loop alias (e.g. "attempt.") inside any string CEL expressions are
// replaced with the canonical "loop." form so the CEL compiler can handle
// them. This is a best-effort, shallow rewrite that touches common expression
// fields (override.when, inputs, CWD, http fields, if conditions, nested
// loops, etc.).
func rewriteFlowStepsForLoopAs(steps []model.FlowStep, as string) []model.FlowStep {
	out := make([]model.FlowStep, 0, len(steps))
	prefix := as + "."
	for _, st := range steps {
		ns := st // copy
		if ns.Override != nil {
			o := *ns.Override
			if o.When != "" {
				o.When = strings.ReplaceAll(o.When, prefix, "loop.")
			}
			if o.Input != nil {
				o.Input = rewriteInInterface(o.Input, prefix)
			}
			ns.Override = &o
		}
		if ns.Loop != nil {
			l := *ns.Loop
			if l.Until != "" {
				l.Until = strings.ReplaceAll(l.Until, prefix, "loop.")
			}
			if s, ok := l.Max.(string); ok {
				l.Max = strings.ReplaceAll(s, prefix, "loop.")
			}
			l.Steps = rewriteFlowStepsForLoopAs(l.Steps, as)
			ns.Loop = &l
		}
		if ns.Parallel != nil {
			p := *ns.Parallel
			p.Steps = rewriteFlowStepsForLoopAs(p.Steps, as)
			ns.Parallel = &p
		}
		if ns.If != nil {
			i := *ns.If
			if i.Condition != "" {
				i.Condition = strings.ReplaceAll(i.Condition, prefix, "loop.")
			}
			i.Then = rewriteFlowStepsForLoopAs(i.Then, as)
			i.Else = rewriteFlowStepsForLoopAs(i.Else, as)
			ns.If = &i
		}
		if ns.Set != nil {
			s := *ns.Set
			newValues := make(map[string]string, len(s.Values))
			for k, v := range s.Values {
				newValues[k] = strings.ReplaceAll(v, prefix, "loop.")
			}
			s.Values = newValues
			ns.Set = &s
		}
		if ns.InlineParticipant != nil {
			p := *ns.InlineParticipant
			p = rewriteParticipantExpressions(p, prefix)
			ns.InlineParticipant = &p
		}
		out = append(out, ns)
	}
	return out
}

func rewriteInInterface(v any, prefix string) any {
	switch v := v.(type) {
	case string:
		return strings.ReplaceAll(v, prefix, "loop.")
	case map[string]interface{}:
		m := make(map[string]interface{}, len(v))
		for k, val := range v {
			m[k] = rewriteInInterface(val, prefix)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(v))
		for i, val := range v {
			s[i] = rewriteInInterface(val, prefix)
		}
		return s
	case map[string]string:
		m := make(map[string]string, len(v))
		for k, val := range v {
			m[k] = strings.ReplaceAll(val, prefix, "loop.")
		}
		return m
	default:
		return v
	}
}

func rewriteParticipantExpressions(p model.Participant, prefix string) model.Participant {
	p.When = strings.ReplaceAll(p.When, prefix, "loop.")
	p.Input = rewriteInInterface(p.Input, prefix)
	p.Output = rewriteInInterface(p.Output, prefix)
	p.CWD = strings.ReplaceAll(p.CWD, prefix, "loop.")
	p.Run = strings.ReplaceAll(p.Run, prefix, "loop.")
	p.URL = strings.ReplaceAll(p.URL, prefix, "loop.")
	p.Method = strings.ReplaceAll(p.Method, prefix, "loop.")
	if p.Headers != nil {
		nh := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			nh[k] = strings.ReplaceAll(v, prefix, "loop.")
		}
		p.Headers = nh
	}
	p.Body = rewriteInInterface(p.Body, prefix)
	p.Path = strings.ReplaceAll(p.Path, prefix, "loop.")
	p.Server = strings.ReplaceAll(p.Server, prefix, "loop.")
	p.Tool = strings.ReplaceAll(p.Tool, prefix, "loop.")
	p.Event = strings.ReplaceAll(p.Event, prefix, "loop.")
	p.Payload = rewriteInInterface(p.Payload, prefix)
	return p
}
