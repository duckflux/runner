package parser

import (
	"fmt"
	"math"
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
	// Message is a clear description of the failure.
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
	validateFlowSteps(wf.Flow, wf.Participants, celEnv, "flow", "", &errs)

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
		for field, expr := range wf.Output.MapField {
			compileCEL(celEnv, expr, fmt.Sprintf("/output/map/%s", field), &errs)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// validateFlowSteps recursively validates a slice of flow steps.
func validateFlowSteps(steps []model.FlowStep, participants map[string]model.Participant, celEnv *cel.Environment, path string, loopAlias string, errs *ValidationErrors) {
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
			pdef, ok := participants[name]
			if !ok {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath,
					Message: fmt.Sprintf("participant %q referenced in flow does not exist", name),
				})
			}
			validateOnError(step.Override.OnError, participants, stepPath+".onError", errs)
			if step.Override.When != "" {
				compileCEL(celEnv, rewriteLoopAlias(step.Override.When, loopAlias), stepPath+".when", errs)
			}
			if step.Override.Input != nil {
				validateInputCEL(step.Override.Input, stepPath+".input", celEnv, loopAlias, errs)
			}
			if step.Override.Retry != nil && step.Override.Retry.Max <= 0 {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".retry.max",
					Message: "retry.max must be greater than zero",
				})
			}
			if step.Override.Workflow != "" && ok && pdef.Type != model.ParticipantTypeWorkflow {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".workflow",
					Message: "workflow override is only valid for workflow participants",
				})
			}

		case step.Loop != nil:
			loop := step.Loop
			if loop.Until == "" && loop.Max == nil {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".loop",
					Message: "loop must specify at least one of 'until' or 'max'",
				})
			}
			if loop.Until != "" {
				alias := loopAlias
				if loop.As != "" {
					alias = loop.As
				}
				compileCEL(celEnv, rewriteLoopAlias(loop.Until, alias), stepPath+".loop.until", errs)
			}
			if loop.Max != nil {
				switch v := loop.Max.(type) {
				case int:
					if v <= 0 {
						*errs = append(*errs, &ValidationError{
							Field:   stepPath + ".loop.max",
							Message: "loop.max must be greater than zero",
						})
					}
				case int64:
					if v <= 0 {
						*errs = append(*errs, &ValidationError{
							Field:   stepPath + ".loop.max",
							Message: "loop.max must be greater than zero",
						})
					}
				case uint, uint64:
					// always > 0 validated by schema; keep as valid
				case float64:
					if v <= 0 || math.Trunc(v) != v {
						*errs = append(*errs, &ValidationError{
							Field:   stepPath + ".loop.max",
							Message: "loop.max must be a positive integer or CEL expression",
						})
					}
				case string:
					alias := loopAlias
					if loop.As != "" {
						alias = loop.As
					}
					compileCEL(celEnv, rewriteLoopAlias(v, alias), stepPath+".loop.max", errs)
				default:
					*errs = append(*errs, &ValidationError{
						Field:   stepPath + ".loop.max",
						Message: "loop.max must be a positive integer or CEL expression",
					})
				}
			}
			// Validate that an explicit `as` name does not collide with reserved identifiers.
			if loop.As != "" {
				if model.IsReservedName(loop.As) {
					*errs = append(*errs, &ValidationError{
						Field:   stepPath + ".loop.as",
						Message: fmt.Sprintf("loop.as name %q is reserved", loop.As),
					})
				}
			}
			bodyAlias := loopAlias
			if loop.As != "" {
				bodyAlias = loop.As
			}
			validateFlowSteps(loop.Steps, participants, celEnv, stepPath+".loop.steps", bodyAlias, errs)

		case step.Parallel != nil:
			for j, sub := range step.Parallel.Steps {
				subPath := fmt.Sprintf("%s.parallel[%d]", stepPath, j)
				// If this is a simple participant reference, validate existence.
				if sub.Participant != "" {
					if _, ok := participants[sub.Participant]; !ok {
						*errs = append(*errs, &ValidationError{
							Field:   subPath,
							Message: fmt.Sprintf("participant %q referenced in parallel does not exist", sub.Participant),
						})
					}
					continue
				}
				// Otherwise validate the nested flow step recursively.
				validateFlowSteps([]model.FlowStep{sub}, participants, celEnv, subPath, loopAlias, errs)
			}

		case step.If != nil:
			ifStep := step.If
			if ifStep.Condition != "" {
				compileCEL(celEnv, rewriteLoopAlias(ifStep.Condition, loopAlias), stepPath+".if", errs)
			}
			validateFlowSteps(ifStep.Then, participants, celEnv, stepPath+".then", loopAlias, errs)
			validateFlowSteps(ifStep.Else, participants, celEnv, stepPath+".else", loopAlias, errs)

		case step.Wait != nil:
			w := step.Wait
			// A wait step must specify at least one of: event, until (CEL) or timeout.
			if w.Event == "" && w.Until == "" && w.Timeout == nil {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".wait",
					Message: "wait must specify at least one of 'event', 'until' or 'timeout'",
				})
			}
			if w.Until != "" {
				compileCEL(celEnv, rewriteLoopAlias(w.Until, loopAlias), stepPath+".wait.until", errs)
			}
			// match may be a CEL expression too; compile if present
			if w.Match != "" {
				compileCEL(celEnv, rewriteLoopAlias(w.Match, loopAlias), stepPath+".wait.match", errs)
			}
			// onTimeout must be one of built-in actions or a participant name; reuse validateOnError
			validateOnError(w.OnTimeout, participants, stepPath+".wait.onTimeout", errs)

		case step.InlineParticipant != nil:
			p := step.InlineParticipant
			if p.As == "" {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".as",
					Message: "inline participant must define 'as'",
				})
			} else if model.IsReservedName(p.As) {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".as",
					Message: fmt.Sprintf("inline participant name %q is reserved", p.As),
				})
			}
			validateOnError(p.OnError, participants, stepPath+".onError", errs)
			if p.When != "" {
				compileCEL(celEnv, rewriteLoopAlias(p.When, loopAlias), stepPath+".when", errs)
			}
			if p.Retry != nil && p.Retry.Max <= 0 {
				*errs = append(*errs, &ValidationError{
					Field:   stepPath + ".retry.max",
					Message: "retry.max must be greater than zero",
				})
			}
			validateInputCEL(p.Input, stepPath+".input", celEnv, loopAlias, errs)
			if p.Type == model.ParticipantTypeEmit {
				validateEmitPayloadCEL(p.Payload, stepPath+".payload", celEnv, loopAlias, errs)
			}
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
	validateInputCEL(p.Input, base+"/input", celEnv, "", errs)

	// Emit participant: validate event and payload expressions.
	if p.Type == model.ParticipantTypeEmit {
		if p.Event == "" {
			*errs = append(*errs, &ValidationError{
				Field:   base + "/event",
				Message: "emit participant must specify an event",
			})
		}
		validateEmitPayloadCEL(p.Payload, base+"/payload", celEnv, "", errs)
	}
}

// validateInputCEL recursively compiles CEL expressions within an input value.
// String values are treated as CEL expressions; map values recurse into each entry.
func validateInputCEL(raw interface{}, path string, celEnv *cel.Environment, loopAlias string, errs *ValidationErrors) {
	switch v := raw.(type) {
	case string:
		compileCEL(celEnv, rewriteLoopAlias(v, loopAlias), path, errs)
	case map[string]interface{}:
		for field, val := range v {
			validateInputCEL(val, fmt.Sprintf("%s/%s", path, field), celEnv, loopAlias, errs)
		}
	}
}

func validateEmitPayloadCEL(raw interface{}, path string, celEnv *cel.Environment, loopAlias string, errs *ValidationErrors) {
	switch v := raw.(type) {
	case string:
		// Emit payload accepts CEL expressions, but literals are also allowed for usability.
		compileCELMaybeLiteral(celEnv, rewriteLoopAlias(v, loopAlias))
	case map[string]interface{}:
		for field, val := range v {
			validateEmitPayloadCEL(val, fmt.Sprintf("%s/%s", path, field), celEnv, loopAlias, errs)
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

func compileCELMaybeLiteral(celEnv *cel.Environment, expr string) {
	if _, err := celEnv.Compile(expr); err != nil {
		return
	}
}

func rewriteLoopAlias(expr string, alias string) string {
	if alias == "" || alias == "loop" {
		return expr
	}
	return strings.ReplaceAll(expr, alias+".", "loop.")
}
