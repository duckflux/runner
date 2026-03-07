package model

// ParticipantType is the type of a participant.
type ParticipantType string

const (
	ParticipantTypeExec     ParticipantType = "exec"
	ParticipantTypeHTTP     ParticipantType = "http"
	ParticipantTypeHuman    ParticipantType = "human"
	ParticipantTypeWorkflow ParticipantType = "workflow"
	ParticipantTypeAgent    ParticipantType = "agent"
	ParticipantTypeMCP      ParticipantType = "mcp"
	ParticipantTypeHook     ParticipantType = "hook"
)

// Participant defines a named step that can be referenced in the flow.
type Participant struct {
	Type    ParticipantType `yaml:"type"`
	As      string          `yaml:"as,omitempty"`
	Timeout *Duration       `yaml:"timeout,omitempty"`
	OnError string          `yaml:"onError,omitempty"`
	Retry   *RetryConfig    `yaml:"retry,omitempty"`
	CWD     string          `yaml:"cwd,omitempty"`
	Input   interface{}     `yaml:"input,omitempty"`
	Output  interface{}     `yaml:"output,omitempty"`

	// exec
	Run string `yaml:"run,omitempty"`

	// http
	URL     string            `yaml:"url,omitempty"`
	Method  string            `yaml:"method,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    interface{}       `yaml:"body,omitempty"`

	// workflow
	Path string `yaml:"path,omitempty"`

	// agent
	Model string   `yaml:"model,omitempty"`
	Tools []string `yaml:"tools,omitempty"`

	// human
	Prompt string `yaml:"prompt,omitempty"`

	// mcp
	Server    string `yaml:"server,omitempty"`
	Operation string `yaml:"operation,omitempty"`
}
