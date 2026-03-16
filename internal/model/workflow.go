package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Workflow is the top-level structure of a duckflux workflow definition file.
type Workflow struct {
	ID           string                 `yaml:"id,omitempty"`
	Name         string                 `yaml:"name,omitempty"`
	Version      interface{}            `yaml:"version,omitempty"`
	Defaults     *Defaults              `yaml:"defaults,omitempty"`
	Inputs       map[string]InputField  `yaml:"inputs,omitempty"`
	Participants map[string]Participant `yaml:"participants,omitempty"`
	Flow         []FlowStep             `yaml:"flow"`
	Output       *WorkflowOutput        `yaml:"output,omitempty"`
}

// Defaults holds global defaults applied to all participants when not overridden.
type Defaults struct {
	Timeout *Duration `yaml:"timeout,omitempty"`
	OnError string    `yaml:"onError,omitempty"`
	CWD     string    `yaml:"cwd,omitempty"`
}

// InputField describes a single workflow input parameter.
type InputField struct {
	Type        string        `yaml:"type,omitempty"`
	Required    bool          `yaml:"required,omitempty"`
	Default     interface{}   `yaml:"default,omitempty"`
	Description string        `yaml:"description,omitempty"`
	Format      string        `yaml:"format,omitempty"`
	Enum        []interface{} `yaml:"enum,omitempty"`
	Minimum     *float64      `yaml:"minimum,omitempty"`
	Maximum     *float64      `yaml:"maximum,omitempty"`
	MinLength   *int          `yaml:"minLength,omitempty"`
	MaxLength   *int          `yaml:"maxLength,omitempty"`
	Pattern     string        `yaml:"pattern,omitempty"`
	Items       *InputField   `yaml:"items,omitempty"`
}

// WorkflowOutput defines the output of the workflow.
// It is either a single CEL expression string or a map of field → CEL expression.
type WorkflowOutput struct {
	// Expression is set when the output is a single string value.
	Expression string
	// Map is set when the output is a mapping of field names to CEL expressions.
	Map map[string]string
	// Schema is the optional schema->map style output (spec v0.2)
	Schema map[string]InputField
	// MapField used when output is provided as { schema: {...}, map: {...} }
	MapField map[string]string
}

// UnmarshalYAML implements yaml.Unmarshaler for WorkflowOutput.
func (o *WorkflowOutput) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		o.Expression = value.Value
		return nil
	case yaml.MappingNode:
		// Detect new schema+map structure
		var probe map[string]interface{}
		if err := value.Decode(&probe); err != nil {
			return err
		}
		if _, ok := probe["schema"]; ok || probe["map"] != nil {
			// decode into typed struct
			type outStruct struct {
				Schema map[string]InputField `yaml:"schema,omitempty"`
				Map    map[string]string     `yaml:"map,omitempty"`
			}
			var o2 outStruct
			if err := value.Decode(&o2); err != nil {
				return err
			}
			o.Schema = o2.Schema
			o.MapField = o2.Map
			return nil
		}
		// fallback: plain map[string]string
		m := make(map[string]string)
		if err := value.Decode(&m); err != nil {
			return err
		}
		o.Map = m
		return nil
	default:
		return fmt.Errorf("output must be a string or mapping, got node kind %v", value.Kind)
	}
}

// MarshalYAML implements yaml.Marshaler for WorkflowOutput.
func (o WorkflowOutput) MarshalYAML() (interface{}, error) {
	if o.Expression != "" {
		return o.Expression, nil
	}
	return o.Map, nil
}
