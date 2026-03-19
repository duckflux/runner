package cel

import (
	"sync"
	"time"
)

// WorkflowMeta holds workflow metadata available under the "workflow" variable.
type WorkflowMeta struct {
	ID      string
	Name    string
	Version string
}

// ExecutionMeta holds execution metadata available under the "execution" variable.
type ExecutionMeta struct {
	ID        string
	Number    int64
	StartedAt string
	Status    string
	CWD       string
	Context   map[string]any
}

// StepResult holds the result of a completed participant step.
// It is available in expressions as "<participant-name>.output", "<participant-name>.status", etc.
type StepResult struct {
	Output     any
	Status     string
	Retries    int64
	StartedAt  string
	FinishedAt string
	Duration   string
	Error      string
	// CWD is the effective current working directory used when running exec participants.
	CWD string
}

// LoopContext holds loop iteration variables available as "loop.*" inside a loop body
// (e.g. `loop.index`, `loop.first`). Internally, the runner rewrites "loop." to "_loop."
// before passing expressions to the CEL compiler, because "loop" is a reserved keyword
// in the CEL grammar.
type LoopContext struct {
	Index     int64
	Iteration int64
	First     bool
	Last      bool
}

// State holds all runtime variable data used to evaluate CEL expressions.
// The mu field protects concurrent reads and writes to Steps during parallel
// step execution.
type State struct {
	Workflow  WorkflowMeta
	Execution ExecutionMeta
	// WorkflowInputs holds the workflow-level inputs, accessible as workflow.inputs.*
	WorkflowInputs map[string]any
	// WorkflowOutput holds the resolved workflow output, accessible as workflow.output
	WorkflowOutput any
	// CurrentInput holds the current participant's input (chain + explicit merged),
	// accessible as the CEL variable "input".
	CurrentInput any
	// CurrentOutput holds the current participant's output after execution,
	// accessible as the CEL variable "output".
	CurrentOutput any
	Env           map[string]string
	Steps         map[string]*StepResult
	// EventPayload holds data from an event when running wait.event steps.
	// It is set by runWait after a matching event is received from the hub.
	EventPayload any
	// Now is injected by the engine as a timestamp for expressions.
	Now  time.Time
	Loop *LoopContext
	mu   sync.RWMutex
}

// SetStep stores a step result thread-safely. It is the only permitted way to
// write to Steps so that parallel goroutines do not race on the map.
func (s *State) SetStep(name string, result *StepResult) {
	s.mu.Lock()
	s.Steps[name] = result
	s.mu.Unlock()
}

