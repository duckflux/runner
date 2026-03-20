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
	// InlineParticipant is set when a full participant definition appears
	// inline inside the flow (has fields like `as` and `type`).
	InlineParticipant *Participant
	// Wait is set for a wait control construct, e.g. `- wait: {...}`.
	Wait *WaitStep
	// Set is set for a set control construct, e.g. `- set: {key: expr}`.
	Set *SetStep
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
			var steps []FlowStep
			if err := value.Content[1].Decode(&steps); err != nil {
				return fmt.Errorf("decoding parallel step: %w", err)
			}
			f.Parallel = &ParallelStep{Steps: steps}
			return nil

		case "if":
			// Support two styles:
			// 1) Legacy top-level keys: `if: "expr"` with sibling `then:`/`else:` keys
			// 2) New nested form: `if: { condition: "expr", then: [...], else: [...] }`
			// Detect presence of sibling `then`/`else` keys to choose decode strategy.
			hasSiblingThen := false
			for i := 0; i < len(value.Content); i += 2 {
				k := value.Content[i].Value
				if k == "then" || k == "else" {
					hasSiblingThen = true
					break
				}
			}
			if hasSiblingThen {
				// legacy form: decode the whole mapping
				var raw struct {
					If        string     `yaml:"if,omitempty"`
					Condition string     `yaml:"condition,omitempty"`
					Then      []FlowStep `yaml:"then"`
					Else      []FlowStep `yaml:"else,omitempty"`
				}
				if err := value.Decode(&raw); err != nil {
					return fmt.Errorf("decoding if step (legacy): %w", err)
				}
				cond := raw.Condition
				if cond == "" {
					cond = raw.If
				}
				f.If = &IfStep{Condition: cond, Then: raw.Then, Else: raw.Else}
				return nil
			}
			// Otherwise, new nested form: decode the value of the `if` key
			node := value.Content[1]
			if node.Kind == yaml.ScalarNode {
				var s string
				if err := node.Decode(&s); err != nil {
					return fmt.Errorf("decoding if step (scalar): %w", err)
				}
				f.If = &IfStep{Condition: s}
				return nil
			}
			var rawNew struct {
				Condition string     `yaml:"condition,omitempty"`
				Then      []FlowStep `yaml:"then"`
				Else      []FlowStep `yaml:"else,omitempty"`
			}
			if err := node.Decode(&rawNew); err != nil {
				return fmt.Errorf("decoding if step (mapping): %w", err)
			}
			f.If = &IfStep{Condition: rawNew.Condition, Then: rawNew.Then, Else: rawNew.Else}
			return nil

		case "wait":
			w := &WaitStep{}
			if err := value.Content[1].Decode(w); err != nil {
				return fmt.Errorf("decoding wait step: %w", err)
			}
			f.Wait = w
			return nil

		case "set":
			var values map[string]string
			if err := value.Content[1].Decode(&values); err != nil {
				return fmt.Errorf("decoding set step: %w", err)
			}
			f.Set = &SetStep{Values: values}
			return nil

		// Detect inline participant definitions: mappings that contain a `type`
		// field (and optionally `as`). These are full participant defs used
		// inline in the flow rather than a named override.
		default:
			// Heuristic: if the mapping contains a `type` key, treat it as an
			// inline participant. Otherwise fall back to participant override
			// where the map key is the participant name.
			hasType := false
			for i := 0; i < len(value.Content); i += 2 {
				if value.Content[i].Value == "type" {
					hasType = true
					break
				}
			}
			if hasType {
				p := &Participant{}
				if err := value.Decode(p); err != nil {
					return fmt.Errorf("decoding inline participant: %w", err)
				}
				f.InlineParticipant = p
				return nil
			}

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
	// As optionally renames the loop context variable (e.g. `as: attempt`).
	As    string      `yaml:"as,omitempty"`
	Until string      `yaml:"until,omitempty"`
	Max   interface{} `yaml:"max,omitempty"`
	Steps []FlowStep  `yaml:"steps"`
}

// WaitStep represents the `wait` control construct.
type WaitStep struct {
	Event     string    `yaml:"event,omitempty"`
	Match     string    `yaml:"match,omitempty"`
	Until     string    `yaml:"until,omitempty"`
	Poll      *Duration `yaml:"poll,omitempty"`
	Timeout   *Duration `yaml:"timeout,omitempty"`
	OnTimeout string    `yaml:"onTimeout,omitempty"`
}

// ParallelStep runs a set of participant steps concurrently.
type ParallelStep struct {
	Steps []FlowStep
}

// IfStep evaluates a CEL condition and routes to the then or else branch.
type IfStep struct {
	Condition string     `yaml:"condition"`
	Then      []FlowStep `yaml:"then"`
	Else      []FlowStep `yaml:"else,omitempty"`
}

// SetStep assigns values to execution.context via CEL expressions.
type SetStep struct {
	// Values maps context keys to CEL expressions.
	Values map[string]string
}

// ParticipantOverrideStep invokes a participant with optional per-invocation overrides.
type ParticipantOverrideStep struct {
	Participant string       `yaml:"-"`
	When        string       `yaml:"when,omitempty"`
	Timeout     *Duration    `yaml:"timeout,omitempty"`
	OnError     string       `yaml:"onError,omitempty"`
	Retry       *RetryConfig `yaml:"retry,omitempty"`
	Input       interface{}  `yaml:"input,omitempty"`
	Workflow    string       `yaml:"workflow,omitempty"`
}
