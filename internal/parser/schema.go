package parser

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	embeddedschema "github.com/duckflux/runner/schema"
)

// schemaURL is the canonical URI used when loading the embedded schema.
const schemaURL = "https://duckflux.dev/schema/v0.2/duckflux.schema.json"

var (
	schemaOnce sync.Once
	compiled   *jsonschema.Schema
	schemaErr  error
)

// loadSchema compiles the embedded duckflux.schema.json exactly once in a
// concurrency-safe manner.
func loadSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		c := jsonschema.NewCompiler()

		var schemaDoc any
		if err := json.Unmarshal(embeddedschema.JSON, &schemaDoc); err != nil {
			schemaErr = fmt.Errorf("parsing embedded schema: %w", err)
			return
		}
		if err := c.AddResource(schemaURL, schemaDoc); err != nil {
			schemaErr = fmt.Errorf("loading embedded schema: %w", err)
			return
		}

		sch, err := c.Compile(schemaURL)
		if err != nil {
			schemaErr = fmt.Errorf("compiling embedded schema: %w", err)
			return
		}
		compiled = sch
	})
	return compiled, schemaErr
}

// validateSchema converts rawYAML bytes to a JSON-compatible document and
// validates it against the embedded duckflux JSON Schema.
// It returns a ValidationErrors slice (satisfying the error interface) when
// the document fails validation, or a plain error for infrastructure failures.
func validateSchema(rawYAML []byte) error {
	// Decode YAML → generic map (yaml.v3 produces map[string]any).
	var doc any
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return &ValidationError{Message: fmt.Sprintf("YAML syntax error: %s", err), cause: err}
	}

	// yaml.v3 uses map[string]interface{} for objects, which JSON Schema
	// validation expects.  We round-trip through JSON to normalise any
	// map[interface{}]interface{} nodes that older decoders sometimes produce.
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("converting YAML to JSON for schema validation: %w", err)
	}
	var jsonDoc any
	if err := json.Unmarshal(jsonBytes, &jsonDoc); err != nil {
		return fmt.Errorf("re-parsing JSON for schema validation: %w", err)
	}

	sch, err := loadSchema()
	if err != nil {
		return err
	}

	if err := sch.Validate(jsonDoc); err != nil {
		return schemaErrorsToValidation(err)
	}
	return nil
}

// schemaErrorsToValidation converts a jsonschema validation error tree into
// our ValidationErrors type.
func schemaErrorsToValidation(err error) ValidationErrors {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return ValidationErrors{&ValidationError{Message: err.Error()}}
	}

	var result ValidationErrors
	collectErrors(ve, &result)
	return result
}

// collectErrors recursively walks the jsonschema error tree and appends leaf
// errors to result.
func collectErrors(ve *jsonschema.ValidationError, result *ValidationErrors) {
	if len(ve.Causes) == 0 {
		// Build a JSON-pointer style location from InstanceLocation segments.
		loc := "/" + strings.Join(ve.InstanceLocation, "/")
		if loc == "/" {
			loc = ""
		}
		*result = append(*result, &ValidationError{
			Field:   loc,
			Message: ve.Error(),
		})
		return
	}
	for _, cause := range ve.Causes {
		collectErrors(cause, result)
	}
}
