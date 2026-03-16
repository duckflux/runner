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
	Input     map[string]any
	Env       map[string]string
	Steps     map[string]*StepResult
	// EventPayload holds data from an event when running wait/emit related steps.
	EventPayload any
	// Now is injected by the engine as a timestamp for expressions.
	Now  time.Time
	Loop *LoopContext
	mu   sync.RWMutex

	eventMu   sync.RWMutex
	eventSubs map[int]chan EventEnvelope
	eventLog  []EventEnvelope
	nextSubID int
}

// SetStep stores a step result thread-safely. It is the only permitted way to
// write to Steps so that parallel goroutines do not race on the map.
func (s *State) SetStep(name string, result *StepResult) {
	s.mu.Lock()
	s.Steps[name] = result
	s.mu.Unlock()
}

// EventEnvelope is a published event instance.
type EventEnvelope struct {
	Name    string
	Payload any
}

// SubscribeEvents creates a new event subscription channel.
func (s *State) SubscribeEvents() (int, <-chan EventEnvelope) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if s.eventSubs == nil {
		s.eventSubs = make(map[int]chan EventEnvelope)
	}
	s.nextSubID++
	id := s.nextSubID
	size := len(s.eventLog)
	if size < 16 {
		size = 16
	}
	ch := make(chan EventEnvelope, size)
	for _, evt := range s.eventLog {
		ch <- evt
	}
	s.eventSubs[id] = ch
	return id, ch
}

// UnsubscribeEvents removes and closes a previously registered event subscriber.
func (s *State) UnsubscribeEvents(id int) {
	s.eventMu.Lock()
	ch, ok := s.eventSubs[id]
	if ok {
		delete(s.eventSubs, id)
		close(ch)
	}
	s.eventMu.Unlock()
}

// PublishEvent fan-outs an event to all active subscribers.
func (s *State) PublishEvent(name string, payload any) {
	s.eventMu.Lock()
	evt := EventEnvelope{Name: name, Payload: payload}
	s.eventLog = append(s.eventLog, evt)
	for _, ch := range s.eventSubs {
		select {
		case ch <- evt:
		default:
			// Best effort: if subscriber channel is full, drop to avoid blocking step execution.
		}
	}
	s.eventMu.Unlock()
}
