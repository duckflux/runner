package parser

import (
	"fmt"
	"strings"
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
