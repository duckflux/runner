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

		expectedType := field.Type
		if expectedType == "" {
			expectedType = "string"
		}

		if e := checkInputType(name, val, expectedType); e != nil {
			errs = append(errs, e)
			continue
		}
		normalized := normalizeInputValue(expectedType, val)

		if field.Format != "" {
			if s, ok := normalized.(string); ok {
				if e := checkStringFormat(name, s, field.Format); e != nil {
					errs = append(errs, e)
				}
			}
		}
		if e := checkInputConstraints(name, normalized, field); e != nil {
			errs = append(errs, e)
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
	case "array":
		if _, ok := val.([]any); !ok {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be an array, got %T", name, val),
			}
		}
	case "object":
		if _, ok := val.(map[string]any); !ok {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be an object, got %T", name, val),
			}
		}
	}

	return nil
}

func normalizeInputValue(expectedType string, val any) any {
	switch expectedType {
	case "boolean":
		if s, ok := val.(string); ok {
			return s == "true"
		}
	case "integer":
		switch v := val.(type) {
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				return parsed
			}
		case float64:
			if math.Trunc(v) == v {
				return int64(v)
			}
		case float32:
			f := float64(v)
			if math.Trunc(f) == f {
				return int64(f)
			}
		}
	case "number":
		if s, ok := val.(string); ok {
			if parsed, err := strconv.ParseFloat(s, 64); err == nil {
				return parsed
			}
		}
	}
	return val
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

func checkInputConstraints(name string, val any, field model.InputField) *ValidationError {
	path := fmt.Sprintf("/inputs/%s", name)

	if len(field.Enum) > 0 {
		found := false
		for _, allowed := range field.Enum {
			if fmt.Sprint(allowed) == fmt.Sprint(val) {
				found = true
				break
			}
		}
		if !found {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be one of %v, got %v", name, field.Enum, val),
			}
		}
	}

	if n, ok := numericValue(val); ok {
		if field.Minimum != nil && n < *field.Minimum {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be >= %v, got %v", name, *field.Minimum, n),
			}
		}
		if field.Maximum != nil && n > *field.Maximum {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q must be <= %v, got %v", name, *field.Maximum, n),
			}
		}
	}

	if s, ok := val.(string); ok {
		if field.MinLength != nil && len(s) < *field.MinLength {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q length must be >= %d", name, *field.MinLength),
			}
		}
		if field.MaxLength != nil && len(s) > *field.MaxLength {
			return &ValidationError{
				Field:   path,
				Message: fmt.Sprintf("input %q length must be <= %d", name, *field.MaxLength),
			}
		}
		if field.Pattern != "" {
			re, err := regexp.Compile(field.Pattern)
			if err != nil {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q has invalid pattern %q: %v", name, field.Pattern, err),
				}
			}
			if !re.MatchString(s) {
				return &ValidationError{
					Field:   path,
					Message: fmt.Sprintf("input %q does not match pattern %q", name, field.Pattern),
				}
			}
		}
	}

	if field.Items != nil {
		if arr, ok := val.([]any); ok {
			itemType := field.Items.Type
			if itemType == "" {
				itemType = "string"
			}
			for i, item := range arr {
				if err := checkInputType(fmt.Sprintf("%s[%d]", name, i), item, itemType); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
