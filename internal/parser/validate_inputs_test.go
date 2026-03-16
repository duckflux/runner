package parser

import (
	"strings"
	"testing"

	"github.com/duckflux/runner/internal/model"
)

// workflowWithInputs returns a minimal workflow with the given input declarations.
func workflowWithInputs(inputs map[string]model.InputField) *model.Workflow {
	return &model.Workflow{
		ID: "test",
		Participants: map[string]model.Participant{
			"stepA": {Type: model.ParticipantTypeExec, Run: "echo x"},
		},
		Flow:   []model.FlowStep{{Participant: "stepA"}},
		Inputs: inputs,
	}
}

// ----- Required field checks -----

func TestValidateInputsRequiredMissing(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"name": {Type: "string", Required: true},
	})
	errs := ValidateInputs(wf, map[string]any{})
	if errs == nil {
		t.Fatal("expected error for missing required input, got nil")
	}
	if !strings.Contains(errs.Error(), "name") {
		t.Errorf("error should mention 'name', got: %v", errs)
	}
	if !strings.Contains(errs.Error(), "required") {
		t.Errorf("error should mention 'required', got: %v", errs)
	}
}

func TestValidateInputsRequiredProvided(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"name": {Type: "string", Required: true},
	})
	errs := ValidateInputs(wf, map[string]any{"name": "alice"})
	if errs != nil {
		t.Errorf("unexpected error: %v", errs)
	}
}

func TestValidateInputsRequiredWithDefault(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"name": {Type: "string", Required: true, Default: "alice"},
	})
	// Even though required=true, a default is present so missing input is OK.
	errs := ValidateInputs(wf, map[string]any{})
	if errs != nil {
		t.Errorf("unexpected error when required field has default: %v", errs)
	}
}

func TestValidateInputsNotRequiredMissing(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"name": {Type: "string", Required: false},
	})
	errs := ValidateInputs(wf, map[string]any{})
	if errs != nil {
		t.Errorf("unexpected error for optional missing input: %v", errs)
	}
}

func TestValidateInputsNoInputsDeclared(t *testing.T) {
	wf := workflowWithInputs(nil)
	errs := ValidateInputs(wf, map[string]any{"extra": "value"})
	if errs != nil {
		t.Errorf("unexpected error when no inputs declared: %v", errs)
	}
}

// ----- String type -----

func TestValidateInputsStringValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"repo": {Type: "string"},
	})
	errs := ValidateInputs(wf, map[string]any{"repo": "https://github.com/example/repo"})
	if errs != nil {
		t.Errorf("unexpected error for valid string: %v", errs)
	}
}

func TestValidateInputsStringWrongType(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"repo": {Type: "string"},
	})
	errs := ValidateInputs(wf, map[string]any{"repo": 42})
	if errs == nil {
		t.Fatal("expected error for non-string value, got nil")
	}
	if !strings.Contains(errs.Error(), "string") {
		t.Errorf("error should mention 'string', got: %v", errs)
	}
}

// ----- Boolean type -----

func TestValidateInputsBooleanNative(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"flag": {Type: "boolean"},
	})
	for _, v := range []any{true, false} {
		errs := ValidateInputs(wf, map[string]any{"flag": v})
		if errs != nil {
			t.Errorf("unexpected error for boolean value %v: %v", v, errs)
		}
	}
}

func TestValidateInputsBooleanStringValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"flag": {Type: "boolean"},
	})
	for _, v := range []string{"true", "false"} {
		errs := ValidateInputs(wf, map[string]any{"flag": v})
		if errs != nil {
			t.Errorf("unexpected error for boolean string %q: %v", v, errs)
		}
	}
}

func TestValidateInputsBooleanStringInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"flag": {Type: "boolean"},
	})
	errs := ValidateInputs(wf, map[string]any{"flag": "yes"})
	if errs == nil {
		t.Fatal("expected error for invalid boolean string, got nil")
	}
	if !strings.Contains(errs.Error(), "boolean") {
		t.Errorf("error should mention 'boolean', got: %v", errs)
	}
}

func TestValidateInputsBooleanWrongType(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"flag": {Type: "boolean"},
	})
	errs := ValidateInputs(wf, map[string]any{"flag": 1})
	if errs == nil {
		t.Fatal("expected error for non-boolean value, got nil")
	}
}

// ----- Integer type -----

func TestValidateInputsIntegerNative(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"count": {Type: "integer"},
	})
	errs := ValidateInputs(wf, map[string]any{"count": 42})
	if errs != nil {
		t.Errorf("unexpected error for native int: %v", errs)
	}
}

func TestValidateInputsIntegerStringValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"count": {Type: "integer"},
	})
	errs := ValidateInputs(wf, map[string]any{"count": "42"})
	if errs != nil {
		t.Errorf("unexpected error for integer string: %v", errs)
	}
}

func TestValidateInputsIntegerStringInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"count": {Type: "integer"},
	})
	errs := ValidateInputs(wf, map[string]any{"count": "three"})
	if errs == nil {
		t.Fatal("expected error for non-integer string, got nil")
	}
	if !strings.Contains(errs.Error(), "integer") {
		t.Errorf("error should mention 'integer', got: %v", errs)
	}
}

func TestValidateInputsIntegerFloat64Whole(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"count": {Type: "integer"},
	})
	// JSON numbers are decoded as float64; whole numbers are still valid integers.
	errs := ValidateInputs(wf, map[string]any{"count": float64(5)})
	if errs != nil {
		t.Errorf("unexpected error for whole-number float64: %v", errs)
	}
}

func TestValidateInputsIntegerFloat64Fractional(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"count": {Type: "integer"},
	})
	errs := ValidateInputs(wf, map[string]any{"count": float64(5.5)})
	if errs == nil {
		t.Fatal("expected error for fractional float64 as integer, got nil")
	}
}

// ----- Number type -----

func TestValidateInputsNumberFloat(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ratio": {Type: "number"},
	})
	errs := ValidateInputs(wf, map[string]any{"ratio": 3.14})
	if errs != nil {
		t.Errorf("unexpected error for float64: %v", errs)
	}
}

func TestValidateInputsNumberInt(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ratio": {Type: "number"},
	})
	errs := ValidateInputs(wf, map[string]any{"ratio": 3})
	if errs != nil {
		t.Errorf("unexpected error for int as number: %v", errs)
	}
}

func TestValidateInputsNumberStringValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ratio": {Type: "number"},
	})
	errs := ValidateInputs(wf, map[string]any{"ratio": "3.14"})
	if errs != nil {
		t.Errorf("unexpected error for parsable number string: %v", errs)
	}
}

func TestValidateInputsNumberStringInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ratio": {Type: "number"},
	})
	errs := ValidateInputs(wf, map[string]any{"ratio": "pi"})
	if errs == nil {
		t.Fatal("expected error for non-numeric string, got nil")
	}
	if !strings.Contains(errs.Error(), "number") {
		t.Errorf("error should mention 'number', got: %v", errs)
	}
}

func TestValidateInputsNumberBoolInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ratio": {Type: "number"},
	})
	errs := ValidateInputs(wf, map[string]any{"ratio": true})
	if errs == nil {
		t.Fatal("expected error for boolean as number, got nil")
	}
}

// ----- Format checks -----

func TestValidateInputsFormatDateValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"dob": {Type: "string", Format: "date"},
	})
	errs := ValidateInputs(wf, map[string]any{"dob": "2024-01-15"})
	if errs != nil {
		t.Errorf("unexpected error for valid date: %v", errs)
	}
}

func TestValidateInputsFormatDateInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"dob": {Type: "string", Format: "date"},
	})
	errs := ValidateInputs(wf, map[string]any{"dob": "15-01-2024"})
	if errs == nil {
		t.Fatal("expected error for invalid date format, got nil")
	}
	if !strings.Contains(errs.Error(), "date") {
		t.Errorf("error should mention 'date', got: %v", errs)
	}
}

func TestValidateInputsFormatDateTimeValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ts": {Type: "string", Format: "date-time"},
	})
	errs := ValidateInputs(wf, map[string]any{"ts": "2024-01-15T10:30:00Z"})
	if errs != nil {
		t.Errorf("unexpected error for valid date-time: %v", errs)
	}
}

func TestValidateInputsFormatDateTimeInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"ts": {Type: "string", Format: "date-time"},
	})
	errs := ValidateInputs(wf, map[string]any{"ts": "2024-01-15"})
	if errs == nil {
		t.Fatal("expected error for invalid date-time format, got nil")
	}
}

func TestValidateInputsFormatURIValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"url": {Type: "string", Format: "uri"},
	})
	errs := ValidateInputs(wf, map[string]any{"url": "https://github.com/duckflux/runner"})
	if errs != nil {
		t.Errorf("unexpected error for valid URI: %v", errs)
	}
}

func TestValidateInputsFormatURIInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"url": {Type: "string", Format: "uri"},
	})
	errs := ValidateInputs(wf, map[string]any{"url": "not a uri"})
	if errs == nil {
		t.Fatal("expected error for invalid URI, got nil")
	}
	if !strings.Contains(errs.Error(), "URI") {
		t.Errorf("error should mention 'URI', got: %v", errs)
	}
}

func TestValidateInputsFormatEmailValid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"contact": {Type: "string", Format: "email"},
	})
	errs := ValidateInputs(wf, map[string]any{"contact": "user@example.com"})
	if errs != nil {
		t.Errorf("unexpected error for valid email: %v", errs)
	}
}

func TestValidateInputsFormatEmailInvalid(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"contact": {Type: "string", Format: "email"},
	})
	errs := ValidateInputs(wf, map[string]any{"contact": "notanemail"})
	if errs == nil {
		t.Fatal("expected error for invalid email, got nil")
	}
	if !strings.Contains(errs.Error(), "email") {
		t.Errorf("error should mention 'email', got: %v", errs)
	}
}

func TestValidateInputsFormatUnknownIgnored(t *testing.T) {
	// Unknown formats should not produce errors.
	wf := workflowWithInputs(map[string]model.InputField{
		"val": {Type: "string", Format: "hostname"},
	})
	errs := ValidateInputs(wf, map[string]any{"val": "my-host"})
	if errs != nil {
		t.Errorf("unknown format should be silently accepted, got: %v", errs)
	}
}

// ----- Multiple errors -----

func TestValidateInputsMultipleErrors(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"name":  {Type: "string", Required: true},
		"count": {Type: "integer", Required: true},
	})
	errs := ValidateInputs(wf, map[string]any{})
	if errs == nil {
		t.Fatal("expected errors for two missing required inputs")
	}
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

// ----- Extra inputs (not declared) are ignored -----

func TestValidateInputsExtraInputIgnored(t *testing.T) {
	wf := workflowWithInputs(map[string]model.InputField{
		"name": {Type: "string"},
	})
	errs := ValidateInputs(wf, map[string]any{"name": "alice", "extra": "ignored"})
	if errs != nil {
		t.Errorf("extra undeclared inputs should be ignored, got: %v", errs)
	}
}
