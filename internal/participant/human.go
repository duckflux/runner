package participant

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/duckflux/runner/internal/model"
)

// HumanParticipant prompts a human operator on stderr and reads one line of
// input from stdin. In non-interactive environments (stdin is not a TTY) it
// returns a clear error rather than blocking forever.
type HumanParticipant struct {
	prompt string
	stdin  io.Reader
	stderr io.Writer
	isTTY  func() bool
}

// NewHuman constructs a HumanParticipant from a participant definition.
// It wires os.Stdin and os.Stderr for production use and detects TTY via
// os.Stdin.Stat().
func NewHuman(def model.Participant) *HumanParticipant {
	return &HumanParticipant{
		prompt: def.Prompt,
		stdin:  os.Stdin,
		stderr: os.Stderr,
		isTTY: func() bool {
			fi, err := os.Stdin.Stat()
			if err != nil {
				return false
			}
			return (fi.Mode() & os.ModeCharDevice) != 0
		},
	}
}

// Execute checks for an interactive terminal, prints the configured prompt to
// stderr, and returns the first line read from stdin. If stdin is not a TTY
// the call fails immediately with a descriptive error. Context cancellation is
// respected: if the context is cancelled while waiting for input the call
// returns ctx.Err().
func (h *HumanParticipant) Execute(ctx context.Context, input any) (any, error) {
	if !h.isTTY() {
		return nil, fmt.Errorf("human participant: requires an interactive terminal (stdin is not a TTY)")
	}

	if h.prompt != "" {
		fmt.Fprint(h.stderr, h.prompt)
	}

	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(h.stdin)
		if scanner.Scan() {
			ch <- result{line: scanner.Text()}
		} else if err := scanner.Err(); err != nil {
			ch <- result{err: fmt.Errorf("human participant: reading input: %w", err)}
		} else {
			ch <- result{err: fmt.Errorf("human participant: stdin closed without input (EOF)")}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("human participant: %w", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.line, nil
	}
}
