package engine

import "github.com/duckflux/runner/internal/model"

// resolveTimeout returns the effective timeout for a step invocation.
// Precedence: flow override > participant definition > workflow defaults > nil (no timeout).
func resolveTimeout(def model.Participant, override *model.ParticipantOverrideStep, wf *model.Workflow) *model.Duration {
	if override != nil && override.Timeout != nil {
		return override.Timeout
	}
	if def.Timeout != nil {
		return def.Timeout
	}
	if wf.Defaults != nil && wf.Defaults.Timeout != nil {
		return wf.Defaults.Timeout
	}
	return nil
}
