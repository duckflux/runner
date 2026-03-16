package participant

import (
	"fmt"

	"github.com/duckflux/runner/internal/model"
)

// BuildRegistry constructs a Registry from the participants map in a workflow
// definition. Each participant definition is instantiated as the appropriate
// Participant implementation.
//
// env is the process-level environment map (env.* values) forwarded to exec
// and workflow participants. runnerFn is the sub-workflow execution callback
// required by WorkflowParticipant; it must not be nil when any participant has
// type "workflow".
func BuildRegistry(wf *model.Workflow, env map[string]string, runnerFn SubWorkflowRunnerFunc) (Registry, error) {
	reg := make(Registry, len(wf.Participants))
	for name, def := range wf.Participants {
		p, err := buildOne(name, def, env, runnerFn)
		if err != nil {
			return nil, fmt.Errorf("building participant %q: %w", name, err)
		}
		reg[name] = p
	}
	return reg, nil
}

// buildOne instantiates a single participant from its definition.
func buildOne(name string, def model.Participant, env map[string]string, runnerFn SubWorkflowRunnerFunc) (Participant, error) {
	switch def.Type {
	case model.ParticipantTypeExec:
		return NewExec(def, env), nil

	case model.ParticipantTypeHTTP:
		return NewHTTP(def, nil), nil

	case model.ParticipantTypeWorkflow:
		return NewWorkflow(def.Path, env, runnerFn)

	case model.ParticipantTypeMCP:
		return NewMCP(def), nil

	case model.ParticipantTypeEmit:
		return NewEmit(def), nil

	default:
		return nil, fmt.Errorf("participant %q: unknown type %q", name, def.Type)
	}
}
