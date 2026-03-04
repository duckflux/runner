package participant

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/duckflux/runner/internal/model"
)

// pipeForTest creates an os.Pipe and registers cleanup to close both ends.
// It returns the read end for use as a blocking stdin substitute.
func pipeForTest(t *testing.T) (io.Reader, *os.File) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		pr.Close()
		pw.Close()
	})
	return pr, pw
}

// newHumanInteractive builds a HumanParticipant wired to the provided
// stdin reader with isTTY returning true (simulates an interactive terminal).
func newHumanInteractive(prompt string, stdin string) *HumanParticipant {
	return &HumanParticipant{
		prompt: prompt,
		stdin:  strings.NewReader(stdin),
		stderr: &bytes.Buffer{},
		isTTY:  func() bool { return true },
	}
}

// newHumanNonInteractive builds a HumanParticipant whose isTTY returns false.
func newHumanNonInteractive() *HumanParticipant {
	return &HumanParticipant{
		prompt: "prompt: ",
		stdin:  strings.NewReader("ignored"),
		stderr: &bytes.Buffer{},
		isTTY:  func() bool { return false },
	}
}

func TestHumanNonInteractiveReturnsError(t *testing.T) {
	p := newHumanNonInteractive()
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error for non-interactive stdin, got nil")
	}
	if !strings.Contains(err.Error(), "TTY") {
		t.Errorf("error = %q; expected it to mention 'TTY'", err.Error())
	}
}

func TestHumanReadsLine(t *testing.T) {
	p := newHumanInteractive("enter value: ", "hello world\n")
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got, ok := out.(string)
	if !ok {
		t.Fatalf("Execute() returned %T, want string", out)
	}
	if got != "hello world" {
		t.Errorf("Execute() = %q, want %q", got, "hello world")
	}
}

func TestHumanPrintsPromptToStderr(t *testing.T) {
	stderrBuf := &bytes.Buffer{}
	p := &HumanParticipant{
		prompt: "Your name: ",
		stdin:  strings.NewReader("Alice\n"),
		stderr: stderrBuf,
		isTTY:  func() bool { return true },
	}
	_, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if stderrBuf.String() != "Your name: " {
		t.Errorf("stderr = %q, want %q", stderrBuf.String(), "Your name: ")
	}
}

func TestHumanEmptyPromptPrintsNothing(t *testing.T) {
	stderrBuf := &bytes.Buffer{}
	p := &HumanParticipant{
		prompt: "",
		stdin:  strings.NewReader("answer\n"),
		stderr: stderrBuf,
		isTTY:  func() bool { return true },
	}
	_, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if stderrBuf.Len() != 0 {
		t.Errorf("stderr = %q, want empty output when prompt is empty", stderrBuf.String())
	}
}

func TestHumanEOFReturnsError(t *testing.T) {
	p := &HumanParticipant{
		prompt: "",
		stdin:  strings.NewReader(""), // immediate EOF
		stderr: &bytes.Buffer{},
		isTTY:  func() bool { return true },
	}
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error on EOF, got nil")
	}
	if !strings.Contains(err.Error(), "EOF") {
		t.Errorf("error = %q; expected it to mention 'EOF'", err.Error())
	}
}

func TestHumanContextCancellation(t *testing.T) {
	// Use a pipe whose read end will block because nothing is written.
	// pipeForTest registers t.Cleanup to close both ends; when the test
	// finishes the write end is closed, which unblocks the goroutine inside
	// Execute so it can drain the buffered result channel and exit cleanly.
	pr, _ := pipeForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := &HumanParticipant{
		prompt: "",
		stdin:  pr,
		stderr: &bytes.Buffer{},
		isTTY:  func() bool { return true },
	}

	start := time.Now()
	_, err := p.Execute(ctx, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Execute() expected error on context cancellation, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Execute() took %v; expected fast cancellation", elapsed)
	}
}

func TestHumanNewHumanUsesModelPrompt(t *testing.T) {
	def := model.Participant{Type: model.ParticipantTypeHuman, Prompt: "Say something: "}
	p := NewHuman(def)
	if p.prompt != "Say something: " {
		t.Errorf("NewHuman prompt = %q, want %q", p.prompt, "Say something: ")
	}
	if p.stdin == nil {
		t.Error("NewHuman stdin is nil")
	}
	if p.stderr == nil {
		t.Error("NewHuman stderr is nil")
	}
	if p.isTTY == nil {
		t.Error("NewHuman isTTY is nil")
	}
}
