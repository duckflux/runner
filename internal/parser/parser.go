package parser

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/duckflux/runner/internal/model"
)

// Parse reads a duckflux workflow definition from r, validates it against the
// embedded JSON Schema, and returns a fully-populated *model.Workflow.
//
// Errors returned are either a ValidationErrors (for schema / YAML problems)
// or a plain error for I/O failures.
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

	return &wf, nil
}
