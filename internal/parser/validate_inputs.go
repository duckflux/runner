package parser

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/duckflux/runner/internal/model"
)

// ValidateInputs validates a map of runtime input values against the workflow's
// declared inputs schema.  Values may originate from --input key=value CLI
// flags (stored as strings) or from an --input-file JSON document (stored as
// typed Go values after JSON unmarshaling).
//
// The following checks are performed for each declared input field:
//   - If required=true and no default is defined, the key must be present.
//   - If the value is provided and a type is declared, the value must be
//     compatible with that type (string, integer, number, boolean).
//   - If the value is a string and a format is declared, the value must match
//     that format (date, date-time, uri, email).
//
// Returns nil when all inputs are valid.
func ValidateInputs(wf *model.Workflow, inputs map[string]any) ValidationErrors {
	var errs ValidationErrors

	for name, field := range wf.Inputs {
		val, provided := inputs[name]

		if field.Required && !provided && field.Default == nil {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("/inputs/%s", name),
				Message: fmt.Sprintf("required input %q was not provided", name),
			})
			continue
		}

		if !provided {
			continue
		}

		if field.Type != "" {
			if e := checkInputType(name, val, field.Type); e != nil {
				errs = append(errs, e)
				continue
			}
		}

		if field.Format != "" {
			if s, ok := val.(string); ok {
				if e := checkStringFormat(name, s, field.Format); e != nil {
					errs = append(errs, e)
				}
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// checkInputType verifies that val is compatible with the declared JSON Schema
// type.  String values (from CLI flags) are accepted for numeric and boolean
// types provided they can be parsed into the target type.
func checkInputType(name string, val any, expectedType string) *ValidationError {
	path := fmt.Sprintf("/inputs/%s", name)

	switch expectedType {
	case "string":
		if _, ok := val.(string); !ok {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a string, got %T", name, val),
			}
		}

	case "boolean":
		switch v := val.(type) {
		case bool:
			// already a boolean — ok
		case string:
			if v != "true" && v != "false" {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q must be a boolean (true or false), got %q", name, v),
				}
			}
		default:
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a boolean, got %T", name, val),
			}
		}

	case "integer":
		switch v := val.(type) {
		case int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64:
			// native integer types — ok
		case float32:
			if v != float32(math.Trunc(float64(v))) {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q must be an integer, got %v", name, val),
				}
			}
		case float64:
			if v != math.Trunc(v) {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q must be an integer, got %v", name, val),
				}
			}
		case string:
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q must be an integer, got %q", name, v),
				}
			}
		default:
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be an integer, got %T", name, val),
			}
		}

	case "number":
		switch v := val.(type) {
		case int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float32, float64:
			// any numeric Go type — ok
		case string:
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q must be a number, got %q", name, v),
				}
			}
		default:
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a number, got %T", name, val),
			}
		}
	}

	return nil
}

// emailRegexp is a basic RFC 5322-like email pattern sufficient for format
// validation.  It is intentionally not exhaustive — the goal is to catch
// obvious non-email strings rather than fully validate RFC 5321 addresses.
var emailRegexp = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// checkStringFormat validates a string value against a declared JSON Schema
// format.  Unknown formats are silently accepted so that future format
// keywords do not break validation.
func checkStringFormat(name, val, format string) *ValidationError {
	path := fmt.Sprintf("/inputs/%s", name)

	switch format {
	case "date":
		if _, err := time.Parse("2006-01-02", val); err != nil {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a date in YYYY-MM-DD format, got %q", name, val),
			}
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, val); err != nil {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a date-time in RFC 3339 format, got %q", name, val),
			}
		}
	case "uri":
		u, err := url.ParseRequestURI(val)
		if err != nil || u.Scheme == "" {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a valid URI, got %q", name, val),
			}
		}
	case "email":
		if !emailRegexp.MatchString(val) {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be a valid email address, got %q", name, val),
			}
		}
	}
	// Unknown formats are silently accepted.
	return nil
}
