package parser

import (
	"fmt"
	"strings"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
)

// ValidationError represents a single structured validation failure returned by
// Parse or ValidateSchema. It carries an optional JSONPath-style location so
// callers can surface useful diagnostics.
type ValidationError struct {
	// Field is the JSON-pointer / dot-path of the offending field, e.g.
	// "/participants/coder/type" or "flow[2].loop".
	Field string
	// Message is a human-readable description of the failure.
	Message string
	// cause is the underlying error, if any.
	cause error
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// Unwrap returns the underlying cause so errors.Is / errors.As traverse the
// chain correctly.
func (e *ValidationError) Unwrap() error {
	return e.cause
}

// ValidationErrors is a collection of ValidationError values that itself
// satisfies the error interface.
type ValidationErrors []*ValidationError

// Error returns all validation messages joined by newlines.
func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// ValidateSemantic performs post-parse semantic validation on a workflow:
//   - participant names must not be reserved identifiers
//   - flow step references must exist in the participants map
//   - onError redirect targets must exist in the participants map
//   - loop steps must specify at least one of until or max > 0
//   - all CEL expressions must compile successfully
//
// celEnv must be a CEL environment built from the same workflow so that
// participant variables are declared when expressions are type-checked.
// If there are no errors the returned slice is nil.
func ValidateSemantic(wf *model.Workflow, celEnv *cel.Environment) ValidationErrors {
	var errs ValidationErrors

	// 1. Reserved name check.
	for name := range wf.Participants {
		if model.IsReservedName(name) {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("/participants/%s", name),
				Message: fmt.Sprintf("participant name %q is reserved", name),
			})
		}
	}

	// 2. Flow step cross-references and CEL expression compilation.
	validateFlowSteps(wf.Flow, wf.Participants, celEnv, "flow", &errs)

	// 3. Participant-level onError redirect targets.
	for name, p := range wf.Participants {
		validateOnError(p.OnError, wf.Participants, fmt.Sprintf("/participants/%s/onError", name), &errs)
		validateParticipantCEL(p, name, celEnv, &errs)
	}

	// 4. Defaults onError redirect target.
	if wf.Defaults != nil {
		validateOnError(wf.Defaults.OnError, wf.Participants, "/defaults/onError", &errs)
	}

	// 5. Workflow output CEL expressions.
	if wf.Output != nil {
		if wf.Output.Expression != "" {
			compileCEL(celEnv, wf.Output.Expression, "/output", &errs)
		}
		for field, expr := range wf.Output.Map {
			compileCEL(celEnv, expr, fmt.Sprintf("/output/%s", field), &errs)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// validateFlowSteps recursively validates a slice of flow steps.
func validateFlowSteps(steps []model.FlowStep, participants map[string]model.Participant, celEnv *cel.Environment, path string, errs *ValidationErrors) {
	for i, step := range steps {
		stepPath := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case step.Participant != "":
			if _, ok := participants[step.Participant]; !ok {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath,
					Message: fmt.Sprintf("participant %q referenced in flow does not exist", step.Participant),
				})
			}

		case step.Override != nil:
			name := step.Override.Participant
			if _, ok := participants[name]; !ok {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath,
					Message: fmt.Sprintf("participant %q referenced in flow does not exist", name),
				})
			}
			validateOnError(step.Override.OnError, participants, stepPath+".onError", errs)
			if step.Override.When != "" {
				compileCEL(celEnv, step.Override.When, stepPath+".when", errs)
			}
			if step.Override.Input != nil {
				validateInputCEL(step.Override.Input, stepPath+".input", celEnv, errs)
			}

		case step.Loop != nil:
			loop := step.Loop
			if loop.Until == "" && loop.Max == 0 {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".loop",
					Message: "loop must specify at least one of 'until' or 'max'",
				})
			}
			if loop.Until != "" {
				compileCEL(celEnv, loop.Until, stepPath+".loop.until", errs)
			}
			validateFlowSteps(loop.Steps, participants, celEnv, stepPath+".loop.steps", errs)

		case step.Parallel != nil:
			for j, name := range step.Parallel.Steps {
				if _, ok := participants[name]; !ok {
					*errs = append(*errs, &ValidationError{
						Field:   fmt.Sprintf("%s.parallel[%d]", stepPath, j),
						Message: fmt.Sprintf("participant %q referenced in parallel does not exist", name),
					})
				}
			}

		case step.If != nil:
			ifStep := step.If
			if ifStep.Condition != "" {
				compileCEL(celEnv, ifStep.Condition, stepPath+".if", errs)
			}
			validateFlowSteps(ifStep.Then, participants, celEnv, stepPath+".then", errs)
			validateFlowSteps(ifStep.Else, participants, celEnv, stepPath+".else", errs)
		}
	}
}

// validateOnError checks that an onError value, if it is a redirect (not a
// built-in action), names an existing participant.
func validateOnError(onError string, participants map[string]model.Participant, path string, errs *ValidationErrors) {
	switch onError {
	case "", "fail", "skip", "retry":
		// built-in actions — always valid
	default:
		if _, ok := participants[onError]; !ok {
			*errs = append(*errs, &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("onError redirect target %q is not a known participant", onError),
			})
		}
	}
}

// validateParticipantCEL compiles CEL expressions embedded in a participant definition.
func validateParticipantCEL(p model.Participant, name string, celEnv *cel.Environment, errs *ValidationErrors) {
	base := fmt.Sprintf("/participants/%s", name)
	validateInputCEL(p.Input, base+"/input", celEnv, errs)
}

// validateInputCEL recursively compiles CEL expressions within an input value.
// String values are treated as CEL expressions; map values recurse into each entry.
func validateInputCEL(raw interface{}, path string, celEnv *cel.Environment, errs *ValidationErrors) {
	switch v := raw.(type) {
	case string:
		compileCEL(celEnv, v, path, errs)
	case map[string]interface{}:
		for field, val := range v {
			validateInputCEL(val, fmt.Sprintf("%s/%s", path, field), celEnv, errs)
		}
	}
}

// compileCEL attempts to compile a CEL expression and appends any error to errs.
func compileCEL(celEnv *cel.Environment, expr string, path string, errs *ValidationErrors) {
	if _, err := celEnv.Compile(expr); err != nil {
		*errs = append(*errs, &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("invalid CEL expression: %s", err),
			cause:   err,
		})
	}
}
