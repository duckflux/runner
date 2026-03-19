package model

// ParticipantType is the type of a participant.
type ParticipantType string

const (
	ParticipantTypeExec     ParticipantType = "exec"
	ParticipantTypeHTTP     ParticipantType = "http"
	ParticipantTypeWorkflow ParticipantType = "workflow"
	ParticipantTypeMCP      ParticipantType = "mcp"
	ParticipantTypeEmit     ParticipantType = "emit"
)

// Participant defines a named step that can be referenced in the flow.
type Participant struct {
	Type    ParticipantType `yaml:"type"`
	As      string          `yaml:"as,omitempty"`
	When    string          `yaml:"when,omitempty"`
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

	// mcp
	Server string `yaml:"server,omitempty"`
	Tool   string `yaml:"tool,omitempty"`

	// emit
	Event     string      `yaml:"event,omitempty"`
	Payload   interface{} `yaml:"payload,omitempty"`
	Ack       bool        `yaml:"ack,omitempty"`
	OnTimeout string      `yaml:"onTimeout,omitempty"`
}
