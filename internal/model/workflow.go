package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Workflow is the top-level structure of a duckflux workflow definition file.
type Workflow struct {
	ID           string                 `yaml:"id"`
	Name         string                 `yaml:"name,omitempty"`
	Version      string                 `yaml:"version,omitempty"`
	Defaults     *Defaults              `yaml:"defaults,omitempty"`
	Inputs       map[string]InputField  `yaml:"inputs,omitempty"`
	Participants map[string]Participant `yaml:"participants"`
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
	Type        string      `yaml:"type,omitempty"`
	Required    bool        `yaml:"required,omitempty"`
	Default     interface{} `yaml:"default,omitempty"`
	Description string      `yaml:"description,omitempty"`
	Format      string      `yaml:"format,omitempty"`
}

// WorkflowOutput defines the output of the workflow.
// It is either a single CEL expression string or a map of field → CEL expression.
type WorkflowOutput struct {
	// Expression is set when the output is a single string value.
	Expression string
	// Map is set when the output is a mapping of field names to CEL expressions.
	Map map[string]string
}

// UnmarshalYAML implements yaml.Unmarshaler for WorkflowOutput.
func (o *WorkflowOutput) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		o.Expression = value.Value
		return nil
	case yaml.MappingNode:
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
