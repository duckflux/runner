package cel

import (
	"fmt"

	"github.com/duckflux/runner/internal/model"
)

// PrecompileWorkflow compiles all CEL expressions used by the workflow runtime
// so execution can reuse cached programs via Environment.Compile.
func (e *Environment) PrecompileWorkflow(wf *model.Workflow) error {
	for name, p := range wf.Participants {
		if err := precompileInputStrict(e, p.Input, fmt.Sprintf("/participants/%s/input", name)); err != nil {
			return err
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
	}

	if err := precompileFlowSteps(e, wf.Flow, "flow"); err != nil {
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
	}

	return nil
}

func precompileFlowSteps(e *Environment, steps []model.FlowStep, path string) error {
	for i, step := range steps {
		stepPath := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case step.Override != nil:
			if step.Override.When != "" {
				if _, err := e.Compile(step.Override.When); err != nil {
					return fmt.Errorf("precompiling %s.when: %w", stepPath, err)
				}
			}
			if err := precompileInputStrict(e, step.Override.Input, stepPath+".input"); err != nil {
				return err
			}
		case step.Loop != nil:
			if step.Loop.Until != "" {
				if _, err := e.Compile(step.Loop.Until); err != nil {
					return fmt.Errorf("precompiling %s.loop.until: %w", stepPath, err)
				}
			}
			if err := precompileFlowSteps(e, step.Loop.Steps, stepPath+".loop.steps"); err != nil {
				return err
			}
		case step.If != nil:
			if step.If.Condition != "" {
				if _, err := e.Compile(step.If.Condition); err != nil {
					return fmt.Errorf("precompiling %s.if: %w", stepPath, err)
				}
			}
			if err := precompileFlowSteps(e, step.If.Then, stepPath+".then"); err != nil {
				return err
			}
			if err := precompileFlowSteps(e, step.If.Else, stepPath+".else"); err != nil {
				return err
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
