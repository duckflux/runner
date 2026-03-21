package cel

import (
	"fmt"
	"strings"

	"github.com/duckflux/runner/internal/model"
)

// PrecompileWorkflow compiles all CEL expressions used by the workflow runtime
// so execution can reuse cached programs via Environment.Compile.
func (e *Environment) PrecompileWorkflow(wf *model.Workflow) error {
	for name, p := range wf.Participants {
		if err := precompileInputStrict(e, p.Input, fmt.Sprintf("/participants/%s/input", name)); err != nil {
			return err
		}
		if p.Type == model.ParticipantTypeExec {
			precompileMaybeCELString(e, p.CWD)
		}
		if p.Type == model.ParticipantTypeHTTP {
			base := fmt.Sprintf("/participants/%s", name)
			precompileMaybeCELString(e, p.URL)
			precompileMaybeCELString(e, p.Method)
			for _, value := range p.Headers {
				precompileMaybeCELString(e, value)
			}
			if err := precompileMaybeCEL(e, p.Body, base+"/body"); err != nil {
				return err
			}
		}
		if p.Type == model.ParticipantTypeEmit {
			base := fmt.Sprintf("/participants/%s/payload", name)
			if err := precompileMaybeCEL(e, p.Payload, base); err != nil {
				return err
			}
		}
	}

	if wf.Defaults != nil {
		precompileMaybeCELString(e, wf.Defaults.CWD)
	}

	if err := precompileFlowSteps(e, wf.Flow, "flow", ""); err != nil {
		return err
	}

	if wf.Output != nil {
		if wf.Output.Expression != "" {
			if _, err := e.Compile(wf.Output.Expression); err != nil {
				return fmt.Errorf("precompiling /output: %w", err)
			}
		}
		for field, expr := range wf.Output.Map {
			if _, err := e.Compile(expr); err != nil {
				return fmt.Errorf("precompiling /output/%s: %w", field, err)
			}
		}
		for field, expr := range wf.Output.MapField {
			if _, err := e.Compile(expr); err != nil {
				return fmt.Errorf("precompiling /output/map/%s: %w", field, err)
			}
		}
	}

	return nil
}

func precompileFlowSteps(e *Environment, steps []model.FlowStep, path string, loopAlias string) error {
	for i, step := range steps {
		stepPath := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case step.Override != nil:
			if step.Override.When != "" {
				if _, err := e.Compile(rewriteLoopAlias(step.Override.When, loopAlias)); err != nil {
					return fmt.Errorf("precompiling %s.when: %w", stepPath, err)
				}
			}
			if err := precompileInputStrict(e, rewriteInInterface(step.Override.Input, loopAlias), stepPath+".input"); err != nil {
				return err
			}
		case step.Loop != nil:
			if step.Loop.Until != "" {
				alias := loopAlias
				if step.Loop.As != "" {
					alias = step.Loop.As
				}
				if _, err := e.Compile(rewriteLoopAlias(step.Loop.Until, alias)); err != nil {
					return fmt.Errorf("precompiling %s.loop.until: %w", stepPath, err)
				}
			}
			if maxExpr, ok := step.Loop.Max.(string); ok {
				alias := loopAlias
				if step.Loop.As != "" {
					alias = step.Loop.As
				}
				if _, err := e.Compile(rewriteLoopAlias(maxExpr, alias)); err != nil {
					return fmt.Errorf("precompiling %s.loop.max: %w", stepPath, err)
				}
			}
			bodyAlias := loopAlias
			if step.Loop.As != "" {
				bodyAlias = step.Loop.As
			}
			if err := precompileFlowSteps(e, step.Loop.Steps, stepPath+".loop.steps", bodyAlias); err != nil {
				return err
			}
		case step.If != nil:
			if step.If.Condition != "" {
				if _, err := e.Compile(rewriteLoopAlias(step.If.Condition, loopAlias)); err != nil {
					return fmt.Errorf("precompiling %s.if: %w", stepPath, err)
				}
			}
			if err := precompileFlowSteps(e, step.If.Then, stepPath+".then", loopAlias); err != nil {
				return err
			}
			if err := precompileFlowSteps(e, step.If.Else, stepPath+".else", loopAlias); err != nil {
				return err
			}
		case step.Wait != nil:
			if step.Wait.Until != "" {
				if _, err := e.Compile(rewriteLoopAlias(step.Wait.Until, loopAlias)); err != nil {
					return fmt.Errorf("precompiling %s.wait.until: %w", stepPath, err)
				}
			}
			if step.Wait.Match != "" {
				if _, err := e.Compile(rewriteLoopAlias(step.Wait.Match, loopAlias)); err != nil {
					return fmt.Errorf("precompiling %s.wait.match: %w", stepPath, err)
				}
			}
		case step.Set != nil:
			for key, expr := range step.Set.Values {
				if _, err := e.Compile(rewriteLoopAlias(expr, loopAlias)); err != nil {
					return fmt.Errorf("precompiling %s.set.%s: %w", stepPath, key, err)
				}
			}
		case step.InlineParticipant != nil:
			p := step.InlineParticipant
			if p.When != "" {
				if _, err := e.Compile(rewriteLoopAlias(p.When, loopAlias)); err != nil {
					return fmt.Errorf("precompiling %s.when: %w", stepPath, err)
				}
			}
			if err := precompileInputStrict(e, rewriteInInterface(p.Input, loopAlias), stepPath+".input"); err != nil {
				return err
			}
			if p.Type == model.ParticipantTypeEmit {
				if err := precompileMaybeCEL(e, rewriteInInterface(p.Payload, loopAlias), stepPath+".payload"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func precompileInputStrict(e *Environment, raw any, path string) error {
	switch v := raw.(type) {
	case string:
		if _, err := e.Compile(v); err != nil {
			return fmt.Errorf("precompiling %s: %w", path, err)
		}
	case map[string]interface{}:
		for field, val := range v {
			if err := precompileInputStrict(e, val, fmt.Sprintf("%s/%s", path, field)); err != nil {
				return err
			}
		}
	}
	return nil
}

func precompileMaybeCELString(e *Environment, expr string) {
	if expr == "" {
		return
	}
	_, _ = e.Compile(expr)
}

func precompileMaybeCEL(e *Environment, raw any, path string) error {
	switch v := raw.(type) {
	case string:
		precompileMaybeCELString(e, v)
	case map[string]interface{}:
		for field, val := range v {
			if err := precompileMaybeCEL(e, val, fmt.Sprintf("%s/%s", path, field)); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, val := range v {
			if err := precompileMaybeCEL(e, val, fmt.Sprintf("%s/%d", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func rewriteLoopAlias(expr string, alias string) string {
	if alias == "" || alias == "loop" {
		return expr
	}
	return strings.ReplaceAll(expr, alias+".", "loop.")
}

func rewriteInInterface(raw any, alias string) any {
	switch v := raw.(type) {
	case string:
		return rewriteLoopAlias(v, alias)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, val := range v {
			out[k] = rewriteInInterface(val, alias)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, val := range v {
			out[i] = rewriteInInterface(val, alias)
		}
		return out
	default:
		return raw
	}
}
