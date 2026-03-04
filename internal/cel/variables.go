package cel

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
	Context   map[string]any
}

// StepResult holds the result of a completed participant step.
// It is available in expressions as "<participant-name>.output", "<participant-name>.status", etc.
type StepResult struct {
	Output  any
	Status  string
	Retries int64
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
type State struct {
	Workflow  WorkflowMeta
	Execution ExecutionMeta
	Input     map[string]any
	Env       map[string]string
	Steps     map[string]*StepResult
	Loop      *LoopContext
}
