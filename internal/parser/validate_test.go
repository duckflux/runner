package parser

import (
	"strings"
	"testing"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
)

// newCELEnv is a helper that builds a CEL environment from a workflow or panics.
func newCELEnv(t *testing.T, wf *model.Workflow) *cel.Environment {
	t.Helper()
	env, err := cel.NewEnv(wf)
	if err != nil {
		t.Fatalf("cel.NewEnv: %v", err)
	}
	return env
}

// minimalWorkflow returns a valid workflow with one exec participant.
func minimalWorkflow() *model.Workflow {
	return &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec, Run: "echo hello"},
		},
		Flow: []model.FlowStep{
			{Participant: "stepA"},
		},
	}
}

// ----- Reserved names -----

func TestValidateSemanticReservedName(t *testing.T) {
	for _, reserved := range model.ReservedNames {
		wf := &model.Workflow{
			ID: "test",
			Participants: map[string]model.Participant{
				reserved: {Type: model.ParticipantTypeExec, Run: "echo x"},
			},
			Flow: []model.FlowStep{{Participant: reserved}},
		}
		env := newCELEnv(t, wf)
		errs := ValidateSemantic(wf, env)
		if errs == nil {
			t.Errorf("reserved name %q: expected validation error, got nil", reserved)
			continue
		}
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, "reserved") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("reserved name %q: expected 'reserved' in error message, got: %v", reserved, errs)
		}
	}
}

func TestValidateSemanticNonReservedName(t *testing.T) {
	wf := minimalWorkflow()
	env := newCELEnv(t, wf)
	if errs := ValidateSemantic(wf, env); errs != nil {
		t.Errorf("unexpected errors for valid workflow: %v", errs)
	}
}

// ----- Flow cross-references -----

func TestValidateSemanticUnknownFlowStep(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Participant: "stepA"},
			{Participant: "unknown"},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for unknown flow step, got nil")
	}
	if !strings.Contains(errs.Error(), "unknown") {
		t.Errorf("error should mention 'unknown', got: %v", errs)
	}
}

func TestValidateSemanticUnknownParallelStep(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Parallel: &model.ParallelStep{Steps: []model.FlowStep{{Participant: "stepA"}, {Participant: "ghost"}}}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for unknown parallel participant, got nil")
	}
	if !strings.Contains(errs.Error(), "ghost") {
		t.Errorf("error should mention 'ghost', got: %v", errs)
	}
}

func TestValidateSemanticOverrideUnknownParticipant(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Override: &model.ParticipantOverrideStep{Participant: "noSuchParticipant"}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for override of unknown participant, got nil")
	}
}

// ----- onError redirect targets -----

func TestValidateSemanticOnErrorInvalidTarget(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec, OnError: "nonexistent"},
		},
		Flow: []model.FlowStep{{Participant: "stepA"}},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid onError redirect, got nil")
	}
	if !strings.Contains(errs.Error(), "nonexistent") {
		t.Errorf("error should mention 'nonexistent', got: %v", errs)
	}
}

func TestValidateSemanticOnErrorBuiltins(t *testing.T) {
	for _, action := range []string{"fail", "skip", "retry", ""} {
		wf := &model.Workflow{
			ID: "test",
			Participants: map[string]model.Participant{
				"stepA": {Type: model.ParticipantTypeExec, OnError: action},
			},
			Flow: []model.FlowStep{{Participant: "stepA"}},
		}
		env := newCELEnv(t, wf)
		errs := ValidateSemantic(wf, env)
		if errs != nil {
			t.Errorf("onError=%q should be valid but got: %v", action, errs)
		}
	}
}

func TestValidateSemanticOnErrorValidRedirect(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA":    {Type: model.ParticipantTypeExec, OnError: "fallback"},
			"fallback": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Participant: "stepA"},
			{Participant: "fallback"},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs != nil {
		t.Errorf("valid onError redirect should pass, got: %v", errs)
	}
}

func TestValidateSemanticDefaultsOnErrorInvalid(t *testing.T) {
	wf := &model.Workflow{
		ID:       "test",
		Defaults: &model.Defaults{OnError: "ghost"},
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "stepA"}},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for defaults.onError redirect to unknown participant")
	}
}

// ----- Loop constraints -----

func TestValidateSemanticLoopNoUntilNoMax(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Steps: []model.FlowStep{{Participant: "stepA"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for loop with no until and no max, got nil")
	}
	if !strings.Contains(errs.Error(), "loop must specify") {
		t.Errorf("expected loop constraint message, got: %v", errs)
	}
}

func TestValidateSemanticLoopWithUntil(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Until: "true",
				Steps: []model.FlowStep{{Participant: "stepA"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs != nil {
		t.Errorf("loop with until should be valid, got: %v", errs)
	}
}

func TestValidateSemanticLoopWithMax(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Max:   5,
				Steps: []model.FlowStep{{Participant: "stepA"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs != nil {
		t.Errorf("loop with max should be valid, got: %v", errs)
	}
}

func TestValidateSemanticLoopUnknownStep(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Max:   3,
				Steps: []model.FlowStep{{Participant: "ghost"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for unknown participant inside loop")
	}
}

// ----- CEL expression compilation -----

func TestValidateSemanticInvalidCELUntil(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Loop: &model.LoopStep{
				Until: "!!!invalid{{",
				Max:   5,
				Steps: []model.FlowStep{{Participant: "stepA"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL expression in loop.until")
	}
	if !strings.Contains(errs.Error(), "invalid CEL expression") {
		t.Errorf("expected 'invalid CEL expression', got: %v", errs)
	}
}

func TestValidateSemanticInvalidCELIf(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: "!!!invalid{{",
				Then:      []model.FlowStep{{Participant: "stepA"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL expression in if condition")
	}
}

func TestValidateSemanticValidCELIf(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{If: &model.IfStep{
				Condition: `workflow.id == "test"`,
				Then:      []model.FlowStep{{Participant: "stepA"}},
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs != nil {
		t.Errorf("valid CEL expression should pass, got: %v", errs)
	}
}

func TestValidateSemanticInvalidCELWhen(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Override: &model.ParticipantOverrideStep{
				Participant: "stepA",
				When:        "!!!bad{{",
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL in when guard")
	}
}

func TestValidateSemanticOutputExpression(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow:   []model.FlowStep{{Participant: "stepA"}},
		Output: &model.WorkflowOutput{Expression: "!!!bad{{"},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid output CEL expression")
	}
}

func TestValidateSemanticOutputMap(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{{Participant: "stepA"}},
		Output: &model.WorkflowOutput{Map: map[string]string{
			"result": `stepA.output`,
			"bad":    "!!!invalid{{",
		}},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL in output map")
	}
}

// ----- Parse integration (semantic errors surfaced through Parse) -----

func TestParseSemanticReservedNameError(t *testing.T) {
	src := `
id: reserved-name-test
participants:
  workflow:
    type: exec
    run: echo x
flow:
  - workflow
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error when participant uses reserved name 'workflow'")
	}
	if !isValidationError(err) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "/participants") {
		t.Errorf("expected participant validation error, got: %v", err)
	}
}

func TestParseSemanticUnknownFlowRef(t *testing.T) {
	src := `
id: bad-ref
participants:
  stepA:
    type: exec
    run: echo x
flow:
  - stepA
  - doesNotExist
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for unknown flow reference")
	}
	if !isValidationError(err) {
		t.Errorf("expected ValidationErrors, got %T: %v", err, err)
	}
}

// ----- CEL validation for participant input (Gap 1) -----

func TestValidateSemanticInvalidCELParticipantInputString(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {
				Type:  model.ParticipantTypeExec,
				Input: "!!!bad{{",
			},
		},
		Flow: []model.FlowStep{{Participant: "stepA"}},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL in participant input, got nil")
	}
	if !strings.Contains(errs.Error(), "invalid CEL expression") {
		t.Errorf("expected 'invalid CEL expression', got: %v", errs)
	}
}

func TestValidateSemanticInvalidCELParticipantInputNestedMap(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {
				Type: model.ParticipantTypeExec,
				Input: map[string]interface{}{
					"nested": map[string]interface{}{
						"deep": "!!!bad{{",
					},
				},
			},
		},
		Flow: []model.FlowStep{{Participant: "stepA"}},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL in nested participant input map, got nil")
	}
	if !strings.Contains(errs.Error(), "invalid CEL expression") {
		t.Errorf("expected 'invalid CEL expression', got: %v", errs)
	}
}

func TestValidateSemanticInvalidCELOverrideInput(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec},
		},
		Flow: []model.FlowStep{
			{Override: &model.ParticipantOverrideStep{
				Participant: "stepA",
				Input:       "!!!bad{{",
			}},
		},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs == nil {
		t.Fatal("expected error for invalid CEL in override input, got nil")
	}
	if !strings.Contains(errs.Error(), "invalid CEL expression") {
		t.Errorf("expected 'invalid CEL expression', got: %v", errs)
	}
}

func TestValidateSemanticValidCELParticipantInput(t *testing.T) {
	wf := &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {
				Type:  model.ParticipantTypeExec,
				Input: `workflow.inputs["branch"]`,
			},
		},
		Flow: []model.FlowStep{{Participant: "stepA"}},
	}
	env := newCELEnv(t, wf)
	errs := ValidateSemantic(wf, env)
	if errs != nil {
		t.Errorf("valid CEL participant input should pass, got: %v", errs)
	}
}
