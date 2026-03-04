package model

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Reserved participant names — cannot be used as participant identifiers.
const (
	ReservedWorkflow  = "workflow"
	ReservedExecution = "execution"
	ReservedInput     = "input"
	ReservedOutput    = "output"
	ReservedEnv       = "env"
	ReservedLoop      = "loop"
)

// ReservedNames is the full list of reserved participant name strings.
var ReservedNames = []string{
	ReservedWorkflow,
	ReservedExecution,
	ReservedInput,
	ReservedOutput,
	ReservedEnv,
	ReservedLoop,
}

// reservedNamesSet is a set used for O(1) reserved name lookups.
var reservedNamesSet = map[string]bool{
	ReservedWorkflow:  true,
	ReservedExecution: true,
	ReservedInput:     true,
	ReservedOutput:    true,
	ReservedEnv:       true,
	ReservedLoop:      true,
}

// IsReservedName reports whether name is a reserved participant identifier.
func IsReservedName(name string) bool {
	return reservedNamesSet[name]
}

// Duration wraps time.Duration and supports YAML unmarshaling of human-readable
// strings like "30s", "5m", "1h".
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// MarshalYAML implements yaml.Marshaler for Duration.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// RetryConfig defines retry behaviour for a participant.
type RetryConfig struct {
	Max     int      `yaml:"max"`
	Backoff Duration `yaml:"backoff,omitempty"`
	Factor  float64  `yaml:"factor,omitempty"`
}
