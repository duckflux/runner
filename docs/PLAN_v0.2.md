# duckflux Runner — Upgrade Tasks (spec v0.2)

This document lists all changes required to align the runner with the spec v0.2.

---

## Summary

The spec v0.2 introduces several breaking changes and new features:

| Category | Changes |
|----------|---------|
| **Schema** | New JSON Schema draft (2020-12), relaxed `required` fields, new participant types |
| **Flow** | Inline participants, `wait` step, nested flow steps in `parallel`, `loop.as` rename, `if` structure change |
| **Participants** | New `emit` type, `tool` field for MCP (replaces `operation`), `hook` removed |
| **Variables** | New `event` reserved name, `now` variable, `<step>.cwd` field |
| **Output** | New `schema`+`map` structure for workflow output |

---

## Phase 1 — Schema & Model Updates

Status: Complete ✅

### 1.1 Update embedded JSON Schema

**File:** `schema/duckflux.schema.json`

- [x] Replace with new schema from spec (draft 2020-12)
- [x] Update `$id` to `https://duckflux.dev/schema/v0.2/duckflux.schema.json`
- [x] Relaxed requirements: only `flow` is required (not `id`, `participants`)

### 1.2 Update `model/workflow.go`

- [x] Make `ID` optional (remove from `required` if validated elsewhere)
- [x] Make `Participants` optional (`map[string]Participant` can be nil)
- [x] Add `version` field support for integer OR string (currently string only)

### 1.3 Update `model/participant.go`

- [x] Add new participant type: `ParticipantTypeEmit = "emit"`
- [x] Remove `ParticipantTypeHook` (no longer in spec)
- [x] Change `Operation` field to `Tool` for MCP participants (`tool` in YAML)
- [x] Add `emit`-specific fields:
  - `Event string` (event name)
  - `Payload interface{}` (CEL expression or map)
  - `Ack bool` (wait for acknowledgment)

### 1.4 Update `model/flow.go`

Major changes required for the union type:

- [ ] Add `InlineParticipant *Participant` field to `FlowStep`
- [ ] Add `Wait *WaitStep` field to `FlowStep`
- [ ] Create new `WaitStep` struct:
  ```go
  type WaitStep struct {
      Event     string    `yaml:"event,omitempty"`
      Match     string    `yaml:"match,omitempty"`
      Until     string    `yaml:"until,omitempty"`
      Poll      *Duration `yaml:"poll,omitempty"`
      Timeout   *Duration `yaml:"timeout,omitempty"`
      OnTimeout string    `yaml:"onTimeout,omitempty"`
  }
  ```
- [ ] Update `LoopStep` to add `As` field for renamed loop context
- [x] Update `ParallelStep` to accept full `FlowStep` slice (not just `[]string`)
- [x] Update `IfStep` structure: change from `if: "expr"` to `if: { condition: "expr", then: [...], else: [...] }`
- [ ] Add `Retry *RetryConfig` to `ParticipantOverrideStep`
- [ ] Add `Workflow string` to `ParticipantOverrideStep` (inline sub-workflow path)
- [ ] Update `UnmarshalYAML` to detect inline participants (has `as` + `type`)

### 1.5 Update `model/common.go`

- [x] Add `ReservedEvent = "event"` to reserved names
- [x] Update `ReservedNames` slice and `reservedNamesSet` map

### 1.6 Update `model/workflow.go` — WorkflowOutput

- [x] Support new structure with `schema` and `map` fields:
  ```go
  type WorkflowOutput struct {
      Expression string
      Map        map[string]string
      Schema     map[string]InputField  // NEW
      MapField   map[string]string      // when using schema+map
  }
  ```
- [x] Update `UnmarshalYAML` to handle all three cases

---

## Phase 2 — Parser Updates

Status: Complete ✅

### 2.1 Update `parser/schema.go`

- [x] Replace embedded schema with new v0.2 schema
- [x] Update schema validation library if needed (draft 2020-12 support)

### 2.2 Update `parser/validate.go`

- [x] Add validation for `wait` steps
- [x] Add validation for inline participants (partial: nested `parallel` validated)
- [x] Add validation for `emit` participants
- [x] Validate `loop.as` doesn't conflict with reserved names
- [x] Validate nested flow steps in `parallel`
- [x] Validate `if.condition` instead of `if` as expression directly

### 2.3 Update `parser/parser.go`

- [x] Handle optional `participants` block (nil map)
- [x] Register inline participants in a synthetic participants map

---

## Phase 3 — CEL Environment Updates

Status: Complete ✅

### 3.1 Update `cel/variables.go`

- [x] Add `EventPayload` field to `State` for wait context
- [x] Add `Now` field to `State` (or inject dynamically)
- [x] Add `CWD string` field to `StepResult`

### 3.2 Update `cel/env.go`

- [x] Declare `event` variable type for wait expressions
- [x] Declare `now` variable as timestamp type (string-based for v1)
- [ ] Support dynamic loop context rename (`loop` → custom `as` name)
- [x] Declare `<step>.cwd` field for exec participants (exposed via step result)

### 3.3 Update loop rewriting

- [ ] Support rewriting custom `as` names (not just `loop.` → `_loop.`)
- [ ] Example: `attempt.index` → `_attempt.index` when `as: attempt`

---

## Phase 4 — Engine Updates

Status: Complete ✅

### 4.1 Update `engine/control.go`

- [x] Update `runLoop` to use `step.Loop.As` for context variable name
- [x] Update `runParallel` to handle full `FlowStep` slices (not just `[]string`)
- [x] Update `runIf` to use `step.If.Condition` instead of direct string

### 4.2 Create `engine/wait.go`

New file for wait step execution:

- [x] Implement `runWait` function
- [x] Support three modes:
 1. **Event mode**: `event` + optional `match` — requires event hub integration (stub for v1)
 2. **Sleep mode**: only `timeout` — simple `time.Sleep`
 3. **Polling mode**: `until` + `poll` + `timeout` — poll condition at intervals
- [x] Handle `onTimeout` (fail/skip/redirect)
- [x] Set `state.EventPayload` when in event mode

### 4.3 Update `engine/sequential.go`

- [x] Add case for `step.Wait != nil` → call `runWait`
- [x] Add case for `step.InlineParticipant != nil` → execute inline participant
- [x] Add `CWD` field to `StepResult` for exec participants
- [x] Inject `now` timestamp before CEL evaluation

### 4.4 Update `engine/state.go`

- [x] Support inline participants in state initialization

---

## Phase 5 — Participant Updates

Status: Complete ✅

### 5.1 Create `participant/emit.go`

New participant type:

 - [x] Create `EmitParticipant` struct
 - [x] Implement `Execute` method:
  - Evaluate `payload` CEL expression(s)
  - Publish to event hub (stub: log and return success)
  - If `ack: true`, wait for acknowledgment (stub: immediate success)
- [ ] Register in participant registry
 - [x] Register in participant registry

### 5.2 Update `participant/mcp.go`

- [x] Rename `Operation` → `Tool` in MCP implementation

### 5.3 Remove `participant/hook.go`

- [x] Delete file (hook is no longer in spec)
- [x] Update registry to not include hook

### 5.4 Update `participant/registry.go`

- [x] Add `emit` to registry
- [x] Remove `hook` from registry

### 5.5 Update `participant/exec.go`

- [x] Return effective `CWD` in result for state tracking

---

## Phase 6 — CLI Updates

Status: Complete ✅

### 6.1 Update input validation

**File:** `cmd/duckflux/run.go` (or equivalent)

- [x] Handle workflows without `participants` block
- [x] Handle inline participants during input resolution

### 6.2 Update lint command

- [x] Validate new constructs (`wait`, inline participants, `emit`)
- [x] Validate `if.condition` path instead of `if` as condition

---

## Phase 7 — Tests & Examples

Status: Complete ✅

### 7.1 Update unit tests

- [x] `model/*_test.go`: Test new YAML unmarshaling for all new types
- [x] `parser/*_test.go`: Test validation of new constructs
- [x] `engine/*_test.go`: Test wait modes, inline participants, loop.as

### 7.2 Update integration tests

- [x] Add test for `wait: { timeout: 1s }` (sleep mode)
- [x] Add test for inline participants
- [x] Add test for `emit` participant (stubbed)
- [x] Add test for `loop: { as: attempt, ... }`

### 7.3 Update example workflows

- [x] Update `examples/code-review.flow.yaml` to use new `if` structure
- [x] Add example with inline participants
- [x] Add example with `emit` and `wait`

---

## Phase 8 — Documentation

Status: Complete ✅

### 8.1 Update README.md

- [x] Document `participants` is now optional
- [x] Document inline participants syntax
- [x] Document `emit` participant type
- [x] Document `wait` flow construct
- [x] Document `loop.as` for renamed context

### 8.2 Update HISTORY.md

- [x] Add entry for spec v0.2 upgrade

---

## Breaking Changes Summary

| Change | Impact |
|--------|--------|
| `if: "expr"` → `if: { condition: "expr", ... }` | Existing workflows with `if` need migration |
| `parallel: [names]` → `parallel: [flowSteps]` | Parallel can now have inline participants |
| `hook` participant removed | Any workflow using `hook` will fail |
| `mcp.operation` → `mcp.tool` | MCP participants need field rename |

---

## Stubs (deferred to v2)

These features are in the spec but will remain stubs:

- [ ] `emit`: Event hub integration (logs only in v1)
- [ ] `wait.event`: Event subscription (returns error in v1)
- [ ] `mcp`: MCP server delegation

---

## Dependency on spec repo

The runner's `schema/duckflux.schema.json` must be synced from `duckflux/spec/duckflux.schema.json`.

Consider adding a script or CI check to verify schema sync.
