package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FlowStep is a union type representing one item in the flow sequence.
// Exactly one field will be set after unmarshaling.
type FlowStep struct {
	// Participant is set for a bare string step reference, e.g. `- stepA`.
	Participant string
	// Loop is set for a loop control construct, e.g. `- loop: {...}`.
	Loop *LoopStep
	// Parallel is set for a parallel construct, e.g. `- parallel: [...]`.
	Parallel *ParallelStep
	// If is set for a conditional branch, e.g. `- if: "expr"\n  then: [...]`.
	If *IfStep
	// Override is set for a participant invocation with overrides, e.g. `- stepA:\n  timeout: 30m`.
	Override *ParticipantOverrideStep
}

// UnmarshalYAML implements yaml.Unmarshaler for FlowStep.
// It inspects the node type and delegates to the appropriate sub-type.
func (f *FlowStep) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// bare participant name reference
		f.Participant = value.Value
		return nil

	case yaml.MappingNode:
		if len(value.Content) == 0 {
			return fmt.Errorf("flow step mapping must not be empty")
		}
		// Inspect the first key to determine the step type.
		firstKey := value.Content[0].Value
		switch firstKey {
		case "loop":
			loop := &LoopStep{}
			if err := value.Content[1].Decode(loop); err != nil {
				return fmt.Errorf("decoding loop step: %w", err)
			}
			f.Loop = loop
			return nil

		case "parallel":
			var steps []string
			if err := value.Content[1].Decode(&steps); err != nil {
				return fmt.Errorf("decoding parallel step: %w", err)
			}
			f.Parallel = &ParallelStep{Steps: steps}
			return nil

		case "if":
			ifStep := &IfStep{}
			if err := value.Decode(ifStep); err != nil {
				return fmt.Errorf("decoding if step: %w", err)
			}
			f.If = ifStep
			return nil

		default:
			// participant override: key is the participant name
			override := &ParticipantOverrideStep{Participant: firstKey}
			if len(value.Content) > 1 {
				if err := value.Content[1].Decode(override); err != nil {
					return fmt.Errorf("decoding override step %q: %w", firstKey, err)
				}
			}
			f.Override = override
			return nil
		}

	default:
		return fmt.Errorf("flow step must be a string or mapping, got node kind %v", value.Kind)
	}
}

// LoopStep repeats a set of steps until a condition is true or a max count is reached.
type LoopStep struct {
	Until string     `yaml:"until,omitempty"`
	Max   int        `yaml:"max,omitempty"`
	Steps []FlowStep `yaml:"steps"`
}

// ParallelStep runs a set of participant steps concurrently.
type ParallelStep struct {
	Steps []string
}

// IfStep evaluates a CEL condition and routes to the then or else branch.
type IfStep struct {
	Condition string     `yaml:"if"`
	Then      []FlowStep `yaml:"then"`
	Else      []FlowStep `yaml:"else,omitempty"`
}

// ParticipantOverrideStep invokes a participant with optional per-invocation overrides.
type ParticipantOverrideStep struct {
	Participant string      `yaml:"-"`
	When        string      `yaml:"when,omitempty"`
	Timeout     *Duration   `yaml:"timeout,omitempty"`
	OnError     string      `yaml:"onError,omitempty"`
	Input       interface{} `yaml:"input,omitempty"`
}
