package parser

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
)

// Parse reads a duckflux workflow definition from r, validates it against the
// embedded JSON Schema, performs semantic validation, and returns a
// fully-populated *model.Workflow.
//
// Errors returned are either a ValidationErrors (for schema / YAML / semantic
// problems) or a plain error for I/O failures.
func Parse(r io.Reader) (*model.Workflow, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading workflow: %w", err)
	}

	// Phase 1 — JSON Schema validation against the raw YAML bytes.
	if err := validateSchema(raw); err != nil {
		return nil, err
	}

	// Phase 2 — Decode into the typed model.
	var wf model.Workflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("YAML decode error: %s", err)}
	}

	// Phase 3 — Semantic validation (cross-references, reserved names, CEL).
	// Register inline participants into a synthetic participant map so that
	// CEL environment and semantic validation see them as declared variables
	// without mutating the original workflow participants map.
	synthetic := make(map[string]model.Participant)
	if wf.Participants != nil {
		for k, v := range wf.Participants {
			synthetic[k] = v
		}
	}
	// Walk the flow to collect inline participants that provide an `as` name.
	collectInlineParticipants(wf.Flow, synthetic)

	// Build a copy of the workflow for validation with the synthetic map.
	wfForValidation := wf
	wfForValidation.Participants = synthetic

	celEnv, err := cel.NewEnv(&wfForValidation)
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}
	if semErrs := ValidateSemantic(&wfForValidation, celEnv); len(semErrs) > 0 {
		return nil, semErrs
	}

	// Persist synthetic participants in the returned workflow so runtime
	// participant registry and CEL bindings can resolve inline steps.
	wf.Participants = synthetic

	return &wf, nil
}

// collectInlineParticipants adds any inline participants with an `as` name to
// the provided map. It recurses into nested flow constructs.
func collectInlineParticipants(steps []model.FlowStep, out map[string]model.Participant) {
	for _, s := range steps {
		if s.InlineParticipant != nil {
			if s.InlineParticipant.As != "" {
				out[s.InlineParticipant.As] = *s.InlineParticipant
			}
			continue
		}
		if s.Override != nil {
			// overrides do not declare participants
			continue
		}
		if s.Loop != nil {
			collectInlineParticipants(s.Loop.Steps, out)
			continue
		}
		if s.Parallel != nil {
			collectInlineParticipants(s.Parallel.Steps, out)
			continue
		}
		if s.If != nil {
			collectInlineParticipants(s.If.Then, out)
			collectInlineParticipants(s.If.Else, out)
			continue
		}
	}
}
