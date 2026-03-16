package participant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/duckflux/runner/internal/model"
)

// ExecParticipant executes a shell command via sh -c, piping input to stdin
// and capturing stdout as the output value. It respects context
// cancellation/timeouts and injects additional environment variables on top of
// the current process environment.
type ExecParticipant struct {
	run      string   // shell command passed to sh -c
	cwd      string   // working directory for command execution (cmd.Dir)
	extraEnv []string // "KEY=VALUE" pairs appended to os.Environ()
}

// NewExec constructs an ExecParticipant from a participant definition and an
// optional map of extra environment variables (e.g. workflow env.* values).
// The extra variables are merged on top of the current process environment.
func NewExec(def model.Participant, extraEnv map[string]string) *ExecParticipant {
	extra := make([]string, 0, len(extraEnv))
	for k, v := range extraEnv {
		extra = append(extra, k+"="+v)
	}
	return &ExecParticipant{
		run:      def.Run,
		cwd:      def.CWD,
		extraEnv: extra,
	}
}

// WithDefinition returns a copy configured from def while preserving the
// environment injection from the receiver.
func (e *ExecParticipant) WithDefinition(def model.Participant) *ExecParticipant {
	extra := make([]string, len(e.extraEnv))
	copy(extra, e.extraEnv)
	return &ExecParticipant{
		run:      def.Run,
		cwd:      def.CWD,
		extraEnv: extra,
	}
}

// Execute runs the configured shell command. If input is non-nil it is written
// to the command's stdin: strings are written verbatim; all other values are
// JSON-marshalled first. Stdout is returned as the output string. If the
// command exits with a non-zero status the error includes the captured stderr.
func (e *ExecParticipant) Execute(ctx context.Context, input any) (any, error) {
	if e.run == "" {
		return nil, fmt.Errorf("exec participant: run command is empty")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", e.run)
	if e.cwd != "" {
		cmd.Dir = e.cwd
	}

	// WaitDelay bounds how long cmd.Wait() may block waiting for I/O goroutines
	// to drain after the process has been killed (e.g. on context cancellation).
	// Without this, child processes that inherit the pipe FDs can keep them open
	// and prevent Wait() from returning until they exit.
	cmd.WaitDelay = 1 * time.Second

	// Environment: current process env augmented with extra variables.
	cmd.Env = append(os.Environ(), e.extraEnv...)

	// Stdin: write input if provided.
	if input != nil {
		stdinData, err := inputToBytes(input)
		if err != nil {
			return nil, fmt.Errorf("exec participant: preparing stdin: %w", err)
		}
		cmd.Stdin = bytes.NewReader(stdinData)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr in the error message to aid debugging.
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("exec participant: %w: %s", err, errMsg)
		}
		return nil, fmt.Errorf("exec participant: %w", err)
	}

	return stdout.String(), nil
}

// inputToBytes converts an Execute input value to bytes for stdin.
// Strings are converted directly; everything else is JSON-marshalled.
func inputToBytes(input any) ([]byte, error) {
	if s, ok := input.(string); ok {
		return []byte(s), nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return data, nil
}
