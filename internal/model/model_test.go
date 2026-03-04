package model

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// ----- Duration -----

func TestDurationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"30s", 30 * time.Second, false},
		{"5m", 5 * time.Minute, false},
		{"1h", time.Hour, false},
		{"2h30m", 2*time.Hour + 30*time.Minute, false},
		{"0s", 0, false},
		{"invalid", 0, true},
		{"notaduration", 0, true},
	}
	for _, tc := range tests {
		var d Duration
		yamlSrc := tc.input
		err := yaml.Unmarshal([]byte(yamlSrc), &d)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Duration.UnmarshalYAML(%q): expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("Duration.UnmarshalYAML(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if d.Duration != tc.want {
			t.Errorf("Duration.UnmarshalYAML(%q): got %v, want %v", tc.input, d.Duration, tc.want)
		}
	}
}

// ----- ReservedNames -----

func TestIsReservedName(t *testing.T) {
	reserved := []string{"workflow", "execution", "input", "output", "env", "loop"}
	for _, name := range reserved {
		if !IsReservedName(name) {
			t.Errorf("IsReservedName(%q) = false, want true", name)
		}
	}
	notReserved := []string{"coder", "reviewer", "deploy", "stepA"}
	for _, name := range notReserved {
		if IsReservedName(name) {
			t.Errorf("IsReservedName(%q) = true, want false", name)
		}
	}
}

// ----- FlowStep unmarshaling -----

func TestFlowStepBareParticipant(t *testing.T) {
	src := `- stepA
- stepB`
	var steps []FlowStep
	if err := yaml.Unmarshal([]byte(src), &steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].Participant != "stepA" {
		t.Errorf("steps[0].Participant = %q, want %q", steps[0].Participant, "stepA")
	}
	if steps[1].Participant != "stepB" {
		t.Errorf("steps[1].Participant = %q, want %q", steps[1].Participant, "stepB")
	}
}

func TestFlowStepLoopStep(t *testing.T) {
	src := `
loop:
  until: "reviewer.output.approved == true"
  max: 5
  steps:
    - coder
    - reviewer
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Loop == nil {
		t.Fatal("expected Loop to be set")
	}
	if step.Loop.Until != "reviewer.output.approved == true" {
		t.Errorf("Loop.Until = %q, want %q", step.Loop.Until, "reviewer.output.approved == true")
	}
	if step.Loop.Max != 5 {
		t.Errorf("Loop.Max = %d, want 5", step.Loop.Max)
	}
	if len(step.Loop.Steps) != 2 {
		t.Fatalf("Loop.Steps len = %d, want 2", len(step.Loop.Steps))
	}
	if step.Loop.Steps[0].Participant != "coder" {
		t.Errorf("Loop.Steps[0].Participant = %q, want %q", step.Loop.Steps[0].Participant, "coder")
	}
}

func TestFlowStepLoopMaxOnly(t *testing.T) {
	src := `
loop:
  max: 3
  steps:
    - stepA
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Loop == nil {
		t.Fatal("expected Loop to be set")
	}
	if step.Loop.Until != "" {
		t.Errorf("Loop.Until = %q, want empty", step.Loop.Until)
	}
	if step.Loop.Max != 3 {
		t.Errorf("Loop.Max = %d, want 3", step.Loop.Max)
	}
}

func TestFlowStepParallelStep(t *testing.T) {
	src := `
parallel:
  - stepA
  - stepB
  - stepC
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Parallel == nil {
		t.Fatal("expected Parallel to be set")
	}
	want := []string{"stepA", "stepB", "stepC"}
	if len(step.Parallel.Steps) != len(want) {
		t.Fatalf("Parallel.Steps len = %d, want %d", len(step.Parallel.Steps), len(want))
	}
	for i, s := range want {
		if step.Parallel.Steps[i] != s {
			t.Errorf("Parallel.Steps[%d] = %q, want %q", i, step.Parallel.Steps[i], s)
		}
	}
}

func TestFlowStepIfStep(t *testing.T) {
	src := `
if: "stepA.output.score > 7"
then:
  - stepB
  - stepC
else:
  - stepD
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.If == nil {
		t.Fatal("expected If to be set")
	}
	if step.If.Condition != "stepA.output.score > 7" {
		t.Errorf("If.Condition = %q, want %q", step.If.Condition, "stepA.output.score > 7")
	}
	if len(step.If.Then) != 2 {
		t.Fatalf("If.Then len = %d, want 2", len(step.If.Then))
	}
	if step.If.Then[0].Participant != "stepB" {
		t.Errorf("If.Then[0].Participant = %q, want %q", step.If.Then[0].Participant, "stepB")
	}
	if len(step.If.Else) != 1 {
		t.Fatalf("If.Else len = %d, want 1", len(step.If.Else))
	}
	if step.If.Else[0].Participant != "stepD" {
		t.Errorf("If.Else[0].Participant = %q, want %q", step.If.Else[0].Participant, "stepD")
	}
}

func TestFlowStepIfNoElse(t *testing.T) {
	src := `
if: "x == true"
then:
  - stepA
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.If == nil {
		t.Fatal("expected If to be set")
	}
	if len(step.If.Else) != 0 {
		t.Errorf("If.Else should be empty, got %d items", len(step.If.Else))
	}
}

func TestFlowStepOverrideStep(t *testing.T) {
	src := `
coder:
  timeout: 30m
  onError: skip
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Override == nil {
		t.Fatal("expected Override to be set")
	}
	if step.Override.Participant != "coder" {
		t.Errorf("Override.Participant = %q, want %q", step.Override.Participant, "coder")
	}
	if step.Override.OnError != "skip" {
		t.Errorf("Override.OnError = %q, want %q", step.Override.OnError, "skip")
	}
	if step.Override.Timeout == nil {
		t.Fatal("Override.Timeout should not be nil")
	}
	if step.Override.Timeout.Duration != 30*time.Minute {
		t.Errorf("Override.Timeout = %v, want 30m", step.Override.Timeout.Duration)
	}
}

func TestFlowStepOverrideWithWhen(t *testing.T) {
	src := `
deploy:
  when: "reviewer.output.approved == true"
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Override == nil {
		t.Fatal("expected Override to be set")
	}
	if step.Override.Participant != "deploy" {
		t.Errorf("Override.Participant = %q, want %q", step.Override.Participant, "deploy")
	}
	if step.Override.When != "reviewer.output.approved == true" {
		t.Errorf("Override.When = %q, want %q", step.Override.When, "reviewer.output.approved == true")
	}
}

func TestFlowStepInvalidKind(t *testing.T) {
	src := `
- - invalid_sequence
`
	var steps []FlowStep
	err := yaml.Unmarshal([]byte(src), &steps)
	if err == nil {
		t.Error("expected error for invalid flow step node kind, got nil")
	}
}

func TestFlowStepEmptyMapping(t *testing.T) {
	src := `{}`
	var step FlowStep
	err := yaml.Unmarshal([]byte(src), &step)
	if err == nil {
		t.Error("expected error for empty mapping, got nil")
	}
}

// ----- WorkflowOutput -----

func TestWorkflowOutputExpression(t *testing.T) {
	src := `reviewer.output.summary`
	var out WorkflowOutput
	if err := yaml.Unmarshal([]byte(src), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Expression != "reviewer.output.summary" {
		t.Errorf("Expression = %q, want %q", out.Expression, "reviewer.output.summary")
	}
	if out.Map != nil {
		t.Errorf("Map should be nil for scalar output")
	}
}

func TestWorkflowOutputInvalidKind(t *testing.T) {
	src := `
- item1
- item2
`
	var out WorkflowOutput
	err := yaml.Unmarshal([]byte(src), &out)
	if err == nil {
		t.Error("expected error for sequence output node, got nil")
	}
}


func TestWorkflowOutputMap(t *testing.T) {
	src := `
approved: reviewer.output.approved
code: coder.output.code
summary: reviewer.output.summary
`
	var out WorkflowOutput
	if err := yaml.Unmarshal([]byte(src), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Expression != "" {
		t.Errorf("Expression should be empty for map output, got %q", out.Expression)
	}
	if out.Map == nil {
		t.Fatal("Map should not be nil")
	}
	if out.Map["approved"] != "reviewer.output.approved" {
		t.Errorf("Map[approved] = %q, want %q", out.Map["approved"], "reviewer.output.approved")
	}
	if out.Map["code"] != "coder.output.code" {
		t.Errorf("Map[code] = %q, want %q", out.Map["code"], "coder.output.code")
	}
}

// ----- Full Workflow unmarshaling -----

func TestWorkflowUnmarshal(t *testing.T) {
	src := `
id: my-workflow
name: My Workflow
version: "1.0"
defaults:
  timeout: 5m
  onError: fail
inputs:
  repoUrl:
    type: string
    required: true
  branch:
    type: string
    default: main
participants:
  coder:
    type: agent
    model: claude-sonnet-4
    timeout: 15m
    onError: retry
    retry:
      max: 3
      backoff: 2s
      factor: 2
  reviewer:
    type: agent
    model: claude-sonnet-4
    onError: fail
flow:
  - coder
  - loop:
      until: "reviewer.output.approved == true"
      max: 5
      steps:
        - coder
        - reviewer
  - reviewer:
      when: "coder.output != ''"
output:
  approved: reviewer.output.approved
  code: coder.output.code
`
	var wf Workflow
	if err := yaml.Unmarshal([]byte(src), &wf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.ID != "my-workflow" {
		t.Errorf("ID = %q, want %q", wf.ID, "my-workflow")
	}
	if wf.Defaults == nil {
		t.Fatal("Defaults should not be nil")
	}
	if wf.Defaults.Timeout == nil || wf.Defaults.Timeout.Duration != 5*time.Minute {
		t.Errorf("Defaults.Timeout = %v, want 5m", wf.Defaults.Timeout)
	}
	if wf.Defaults.OnError != "fail" {
		t.Errorf("Defaults.OnError = %q, want fail", wf.Defaults.OnError)
	}

	// inputs
	if len(wf.Inputs) != 2 {
		t.Fatalf("Inputs len = %d, want 2", len(wf.Inputs))
	}
	if wf.Inputs["repoUrl"].Type != "string" {
		t.Errorf("Inputs[repoUrl].Type = %q, want string", wf.Inputs["repoUrl"].Type)
	}
	if !wf.Inputs["repoUrl"].Required {
		t.Error("Inputs[repoUrl].Required = false, want true")
	}

	// participants
	if len(wf.Participants) != 2 {
		t.Fatalf("Participants len = %d, want 2", len(wf.Participants))
	}
	coder := wf.Participants["coder"]
	if coder.Type != ParticipantTypeAgent {
		t.Errorf("coder.Type = %q, want agent", coder.Type)
	}
	if coder.Retry == nil {
		t.Fatal("coder.Retry should not be nil")
	}
	if coder.Retry.Max != 3 {
		t.Errorf("coder.Retry.Max = %d, want 3", coder.Retry.Max)
	}
	if coder.Retry.Backoff.Duration != 2*time.Second {
		t.Errorf("coder.Retry.Backoff = %v, want 2s", coder.Retry.Backoff.Duration)
	}
	if coder.Retry.Factor != 2 {
		t.Errorf("coder.Retry.Factor = %v, want 2", coder.Retry.Factor)
	}

	// flow
	if len(wf.Flow) != 3 {
		t.Fatalf("Flow len = %d, want 3", len(wf.Flow))
	}
	if wf.Flow[0].Participant != "coder" {
		t.Errorf("Flow[0].Participant = %q, want coder", wf.Flow[0].Participant)
	}
	if wf.Flow[1].Loop == nil {
		t.Fatal("Flow[1].Loop should not be nil")
	}
	if wf.Flow[2].Override == nil {
		t.Fatal("Flow[2].Override should not be nil")
	}

	// output
	if wf.Output == nil {
		t.Fatal("Output should not be nil")
	}
	if wf.Output.Map == nil {
		t.Fatal("Output.Map should not be nil")
	}
	if wf.Output.Map["approved"] != "reviewer.output.approved" {
		t.Errorf("Output.Map[approved] = %q", wf.Output.Map["approved"])
	}
}

func TestWorkflowNullInputs(t *testing.T) {
	src := `
id: simple
participants:
  stepA:
    type: exec
    run: echo hello
inputs:
  repoUrl:
  branch:
flow:
  - stepA
`
	var wf Workflow
	if err := yaml.Unmarshal([]byte(src), &wf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wf.Inputs) != 2 {
		t.Fatalf("Inputs len = %d, want 2", len(wf.Inputs))
	}
	// null inputs decode to zero-value InputField
	if wf.Inputs["repoUrl"].Type != "" {
		t.Errorf("repoUrl.Type = %q, want empty string", wf.Inputs["repoUrl"].Type)
	}
}

func TestFlowStepNestedLoopInIf(t *testing.T) {
	src := `
if: "x == true"
then:
  - loop:
      max: 3
      steps:
        - stepA
else:
  - stepB
`
	var step FlowStep
	if err := yaml.Unmarshal([]byte(src), &step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.If == nil {
		t.Fatal("expected If to be set")
	}
	if len(step.If.Then) != 1 {
		t.Fatalf("If.Then len = %d, want 1", len(step.If.Then))
	}
	if step.If.Then[0].Loop == nil {
		t.Fatal("expected nested Loop in then branch")
	}
	if step.If.Then[0].Loop.Max != 3 {
		t.Errorf("nested Loop.Max = %d, want 3", step.If.Then[0].Loop.Max)
	}
}
