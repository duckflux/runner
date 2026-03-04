package engine

import (
"context"
"errors"
"testing"
"time"

"github.com/duckflux/runner/internal/model"
"github.com/duckflux/runner/internal/participant"
)

// ----- executeWithRetry -----

func TestExecuteWithRetryFirstAttemptSuccess(t *testing.T) {
calls := 0
out, retries, err := executeWithRetry(context.Background(), func() (any, error) {
calls++
return "ok", nil
}, &model.RetryConfig{Max: 3})

if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if out != "ok" {
t.Errorf("output = %v, want ok", out)
}
if retries != 0 {
t.Errorf("retries = %d, want 0", retries)
}
if calls != 1 {
t.Errorf("calls = %d, want 1", calls)
}
}

func TestExecuteWithRetrySucceedsOnSecondAttempt(t *testing.T) {
calls := 0
out, retries, err := executeWithRetry(context.Background(), func() (any, error) {
calls++
if calls < 2 {
return nil, errors.New("transient")
}
return "success", nil
}, &model.RetryConfig{Max: 3, Backoff: model.Duration{Duration: time.Millisecond}})

if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if out != "success" {
t.Errorf("output = %v, want success", out)
}
if retries != 1 {
t.Errorf("retries = %d, want 1", retries)
}
if calls != 2 {
t.Errorf("calls = %d, want 2", calls)
}
}

func TestExecuteWithRetryAllAttemptsExhausted(t *testing.T) {
calls := 0
boom := errors.New("boom")
_, retries, err := executeWithRetry(context.Background(), func() (any, error) {
calls++
return nil, boom
}, &model.RetryConfig{Max: 3, Backoff: model.Duration{Duration: time.Millisecond}})

if !errors.Is(err, boom) {
t.Fatalf("err = %v, want boom", err)
}
if retries != 3 {
t.Errorf("retries = %d, want 3", retries)
}
// 1 initial call + 3 retries = 4 total.
if calls != 4 {
t.Errorf("calls = %d, want 4", calls)
}
}

func TestExecuteWithRetryNilConfigNoRetry(t *testing.T) {
calls := 0
fail := errors.New("fail")
_, retries, err := executeWithRetry(context.Background(), func() (any, error) {
calls++
return nil, fail
}, nil)

if !errors.Is(err, fail) {
t.Fatalf("err = %v, want fail", err)
}
if retries != 0 {
t.Errorf("retries = %d, want 0", retries)
}
if calls != 1 {
t.Errorf("calls = %d, want 1 (no retries)", calls)
}
}

func TestExecuteWithRetryZeroMaxNoRetry(t *testing.T) {
calls := 0
_, _, err := executeWithRetry(context.Background(), func() (any, error) {
calls++
return nil, errors.New("fail")
}, &model.RetryConfig{Max: 0})

if err == nil {
t.Fatal("expected error, got nil")
}
if calls != 1 {
t.Errorf("calls = %d, want 1", calls)
}
}

func TestExecuteWithRetryContextCancelled(t *testing.T) {
ctx, cancel := context.WithCancel(context.Background())
// Cancel immediately so the first retry sleep is interrupted.
cancel()

calls := 0
_, _, err := executeWithRetry(ctx, func() (any, error) {
calls++
return nil, errors.New("fail")
}, &model.RetryConfig{Max: 5, Backoff: model.Duration{Duration: time.Second}})

if !errors.Is(err, context.Canceled) {
t.Errorf("err = %v, want context.Canceled", err)
}
// Only the initial call should have happened before context was checked.
if calls != 1 {
t.Errorf("calls = %d, want 1", calls)
}
}

func TestExecuteWithRetryExponentialBackoff(t *testing.T) {
// Verify that the delay grows between retries by recording timestamps.
calls := 0
var timestamps []time.Time
_, _, _ = executeWithRetry(context.Background(), func() (any, error) {
calls++
timestamps = append(timestamps, time.Now())
return nil, errors.New("fail")
}, &model.RetryConfig{
Max:     2,
Backoff: model.Duration{Duration: 10 * time.Millisecond},
Factor:  2.0,
})

// 1 initial + 2 retries = 3 calls.
if calls != 3 {
t.Fatalf("calls = %d, want 3", calls)
}
// Second gap (backoff * 2^1) should be larger than first gap (backoff * 2^0).
gap0 := timestamps[1].Sub(timestamps[0])
gap1 := timestamps[2].Sub(timestamps[1])
if gap1 <= gap0 {
t.Errorf("expected gap1 (%v) > gap0 (%v) for exponential backoff", gap1, gap0)
}
}

// ----- Run integration tests for retry and redirect -----

func TestRunOnErrorRetrySucceedsAfterRetries(t *testing.T) {
calls := 0
wf := &model.Workflow{
ID: "wf1",
Participants: map[string]model.Participant{
"step1": {
Type:    model.ParticipantTypeExec,
OnError: "retry",
Retry:   &model.RetryConfig{Max: 3, Backoff: model.Duration{Duration: time.Millisecond}},
},
},
Flow: []model.FlowStep{{Participant: "step1"}},
}

p := participantFn(func(_ context.Context, _ any) (any, error) {
calls++
if calls < 3 {
return nil, errors.New("transient")
}
return "recovered", nil
})
reg := participant.Registry{"step1": p}

out, err := Run(context.Background(), wf, nil, nil, reg)
if err != nil {
t.Fatalf("Run() error: %v", err)
}
if out != "recovered" {
t.Errorf("Run() = %v, want recovered", out)
}
if calls != 3 {
t.Errorf("calls = %d, want 3 (1 initial + 2 retries)", calls)
}
}

func TestRunOnErrorRetryExhausted(t *testing.T) {
calls := 0
wf := &model.Workflow{
ID: "wf1",
Participants: map[string]model.Participant{
"step1": {
Type:    model.ParticipantTypeExec,
OnError: "retry",
Retry:   &model.RetryConfig{Max: 2, Backoff: model.Duration{Duration: time.Millisecond}},
},
},
Flow: []model.FlowStep{{Participant: "step1"}},
}
p := participantFn(func(_ context.Context, _ any) (any, error) {
calls++
return nil, errors.New("always fails")
})
reg := participant.Registry{"step1": p}

_, err := Run(context.Background(), wf, nil, nil, reg)
if err == nil {
t.Fatal("Run() expected error after retry exhaustion, got nil")
}
// 1 initial + 2 retries = 3 total calls.
if calls != 3 {
t.Errorf("calls = %d, want 3", calls)
}
}

func TestRunOnErrorRedirectSuccess(t *testing.T) {
wf := &model.Workflow{
ID: "wf1",
Participants: map[string]model.Participant{
"step1":    {Type: model.ParticipantTypeExec, OnError: "fallback"},
"fallback": {Type: model.ParticipantTypeExec},
},
Flow: []model.FlowStep{{Participant: "step1"}},
}
step1 := participantFn(func(_ context.Context, _ any) (any, error) {
return nil, errors.New("step1 failed")
})
fb := participantFn(func(_ context.Context, _ any) (any, error) {
return "fallback-output", nil
})
reg := participant.Registry{"step1": step1, "fallback": fb}

// Redirect should succeed: workflow continues without error.
_, err := Run(context.Background(), wf, nil, nil, reg)
if err != nil {
t.Fatalf("Run() error: %v", err)
}
}

func TestRunOnErrorRedirectFallbackAlsoFails(t *testing.T) {
wf := &model.Workflow{
ID: "wf1",
Participants: map[string]model.Participant{
"step1":    {Type: model.ParticipantTypeExec, OnError: "fallback"},
"fallback": {Type: model.ParticipantTypeExec},
},
Flow: []model.FlowStep{{Participant: "step1"}},
}
step1 := participantFn(func(_ context.Context, _ any) (any, error) {
return nil, errors.New("step1 failed")
})
fb := participantFn(func(_ context.Context, _ any) (any, error) {
return nil, errors.New("fallback also failed")
})
reg := participant.Registry{"step1": step1, "fallback": fb}

_, err := Run(context.Background(), wf, nil, nil, reg)
if err == nil {
t.Fatal("Run() expected error when both step and fallback fail")
}
}

func TestRunRetryWithContextCancellation(t *testing.T) {
ctx, cancel := context.WithCancel(context.Background())
cancel() // cancelled before the first retry sleep

wf := &model.Workflow{
ID: "wf1",
Participants: map[string]model.Participant{
"step1": {
Type:    model.ParticipantTypeExec,
OnError: "retry",
Retry:   &model.RetryConfig{Max: 5, Backoff: model.Duration{Duration: time.Second}},
},
},
Flow: []model.FlowStep{{Participant: "step1"}},
}
p := participantFn(func(_ context.Context, _ any) (any, error) {
return nil, errors.New("always fails")
})
reg := participant.Registry{"step1": p}

_, err := Run(ctx, wf, nil, nil, reg)
if err == nil {
t.Fatal("Run() expected error for cancelled context, got nil")
}
}

// ----- helpers -----

// participantFn adapts a function to the participant.Participant interface.
type participantFn func(ctx context.Context, input any) (any, error)

func (f participantFn) Execute(ctx context.Context, input any) (any, error) {
return f(ctx, input)
}
