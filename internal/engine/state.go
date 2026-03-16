package engine

import (
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/duckflux/runner/internal/cel"
	"github.com/duckflux/runner/internal/model"
)

// NewState creates an initial execution State from a workflow definition and
// the caller-supplied inputs and environment variables. Nil maps are converted
// to empty maps. Workflow input defaults are applied for any field not present
// in inputs.
func NewState(wf *model.Workflow, inputs map[string]any, env map[string]string) *cel.State {
	if inputs == nil {
		inputs = map[string]any{}
	}
	// Apply workflow input defaults for fields that were not provided.
	for name, field := range wf.Inputs {
		if _, ok := inputs[name]; !ok && field.Default != nil {
			inputs[name] = field.Default
		}
	}
	if env == nil {
		env = map[string]string{}
	}
	ver := ""
	if wf.Version != nil {
		ver = fmt.Sprint(wf.Version)
	}
	s := &cel.State{
		Workflow: cel.WorkflowMeta{
			ID:      wf.ID,
			Name:    wf.Name,
			Version: ver,
		},
		Execution: cel.ExecutionMeta{
			ID:        newExecutionID(),
			Number:    1,
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			Status:    "running",
			Context:   map[string]any{},
		},
		Input: inputs,
		Env:   env,
		Steps: make(map[string]*cel.StepResult),
	}
	if cwd, err := os.Getwd(); err == nil {
		s.Execution.CWD = cwd
		s.Execution.Context["cwd"] = cwd
	}

	// Populate Steps map with placeholders for inline participants so that
	// they exist in the state once executed. Traverse the flow to find any
	// inline participant definitions with an `as` name.
	var walk func([]model.FlowStep)
	walk = func(steps []model.FlowStep) {
		for _, st := range steps {
			if st.InlineParticipant != nil && st.InlineParticipant.As != "" {
				if _, ok := s.Steps[st.InlineParticipant.As]; !ok {
					s.Steps[st.InlineParticipant.As] = &cel.StepResult{}
				}
			}
			if st.Loop != nil {
				walk(st.Loop.Steps)
			}
			if st.Parallel != nil {
				walk(st.Parallel.Steps)
			}
			if st.If != nil {
				walk(st.If.Then)
				walk(st.If.Else)
			}
			if st.Override != nil {
				// override participants refer to named participants; no-op
			}
		}
	}
	walk(wf.Flow)

	return s
}

// newExecutionID generates a random UUID-style execution identifier using
// crypto/rand so each workflow execution is uniquely identifiable.
func newExecutionID() string {
	b := make([]byte, 16)
	// crypto/rand.Read always returns nil on all supported platforms (see Go docs);
	// the blank identifier intentionally suppresses the linter warning.
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
