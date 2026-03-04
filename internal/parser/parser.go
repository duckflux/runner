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
	celEnv, err := cel.NewEnv(&wf)
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}
	if semErrs := ValidateSemantic(&wf, celEnv); len(semErrs) > 0 {
		return nil, semErrs
	}

	return &wf, nil
}
