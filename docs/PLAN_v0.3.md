# duckflux Runner — Upgrade Plan (spec v0.3)

Reference inputs for this plan:
- `../spec/CHANGELOG.md` (v0.3 Draft)
- `../spec/SPEC.md` (Version 0.3)
- `../spec/duckflux.schema.json` (`$id` v0.3)

## Goal

Upgrade the Go runner to fully align with spec v0.3 semantics, with focus on:

1. Variable namespace redesign (`workflow.inputs`, `workflow.output`, participant-scoped `input`/`output`)
2. Implicit I/O chain across flow execution (including `if`, `loop`, `parallel` rules)
3. Anonymous inline participants (inline `type` without `as`)
4. Named inline uniqueness validation
5. Schema sync (`v0.3`) and regression coverage
6. `flow` structural contract (`flow` MUST be non-empty)
7. Explicit inline sub-workflow support coverage (`type: workflow` inside `flow`)
8. `emit` acknowledged-timeout behavior (`ack: true` + `timeout` + `onTimeout`)

This plan is written for incremental implementation by a smaller-capacity agent.

---

## Current Gap Summary

| Area | v0.3 requirement | Current runner status | Required work |
|------|------------------|-----------------------|---------------|
| Schema version | `$id` must be `.../v0.3/...` | still `v0.2` | update embedded schema + parser schema URL |
| Inline participants | `as` optional | `as` effectively required by parser/runtime | allow anonymous inline |
| Inline naming | named inline `as` must be unique globally | duplicates overwrite silently | add semantic uniqueness validation |
| Workflow input namespace | workflow inputs accessed via `workflow.inputs.*` | workflows/tests still rely on global `input.*` for workflow inputs | move workflow inputs into `workflow.inputs` in CEL bindings/tests |
| Participant input var | `input` is current participant input (chain+explicit merged) | `input` currently points to workflow inputs map | redesign state + CEL bindings |
| Participant output var | `output` is current participant output | not exposed as CEL variable | add runtime-scoped `output` binding |
| Implicit chain | output of step N is input of step N+1 | not implemented | implement chain engine-wide |
| Chain merge | chain + explicit input merge rules | not implemented | implement strict merge logic |
| Parallel chain result | chained output after `parallel` is ordered array | not implemented | aggregate branch outputs into array |
| Minimal doc | `flow: [{type: exec, run: ...}]` valid | rejected at runtime because inline requires `as` | support anonymous inline execution |
| Non-empty flow | `flow` MUST contain at least one step | enforced by schema, but not explicitly tracked in tests | add parser regression test for `flow: []` rejection |
| Inline sub-workflow mode | sub-workflow can be defined inline in `flow` | behavior implied by generic inline support, but not explicitly covered in plan/tests | add parser/integration tests for inline `type: workflow` |
| `emit` timeout semantics | when `ack: true`, timeout handling uses `onTimeout` (`fail`/`skip`) | emit model/runtime do not expose participant-level `onTimeout`; ack flow is currently stubbed | add implementation decision + tests (or explicitly defer with documented rationale) |

---

## Implementation Principles

1. Keep existing architecture (`model`, `parser`, `cel`, `engine`, `participant`) and avoid broad package reshuffles.
2. Ship in small phases with `go test ./...` at each phase.
3. Prefer explicit runtime context over hidden global behavior for CEL scope.
4. For ambiguous spec corners, use deterministic defaults and document them (see assumptions below).

### Assumptions to unblock implementation

If product direction does not override these, implement as follows:

1. If a step is skipped (`when: false` or `onError: skip`), the implicit chain remains unchanged.
2. `wait` step does not create output; it preserves incoming chain unchanged.
3. `workflow.output` is `null` during execution and is set only after final output resolution.
4. Static merge compatibility checks are best-effort; runtime merge checks are authoritative.

---

## Phase 0 — Baseline and Safety Nets

### Files
- `internal/integration/integration_test.go`
- `internal/engine/engine_test.go`
- `internal/cel/cel_test.go`

### Tasks
- [ ] Run current baseline tests and capture failures after each phase.
- [ ] Add placeholder TODO tests (skipped) for v0.3 chain semantics before refactor, so target behavior is explicit.

### Exit Criteria
- Existing test suite passes before code changes.

---

## Phase 1 — Schema Synchronization (v0.3)

### Files
- `schema/duckflux.schema.json`
- `internal/parser/schema.go`
- `internal/parser/parse_test.go`
- `internal/parser/parser_test.go`

### Tasks
- [ ] Replace local schema with `spec/duckflux.schema.json` v0.3.
- [ ] Update `schemaURL` constant from v0.2 to v0.3 in parser.
- [ ] Add parse test for minimal anonymous inline workflow:
  ```yaml
  flow:
    - type: exec
      run: echo "ok"
  ```
- [ ] Add parser test asserting removed participant types (`agent`, `human`) are rejected by schema validation.
- [ ] Add parser test asserting `flow: []` fails schema validation (`minItems: 1` contract).

### Exit Criteria
- Schema/lint accepts anonymous inline step.
- Schema/lint rejects `agent` and `human` types.

---

## Phase 2 — Parser Semantics for Inline Naming

### Files
- `internal/parser/validate.go`
- `internal/parser/parser.go`
- `internal/parser/validate_test.go`
- `internal/parser/parse_test.go`

### Tasks
- [ ] Remove semantic rule that inline participants must define `as`.
- [ ] Add semantic validation for named inline uniqueness:
  - [ ] conflict with top-level `participants` keys
  - [ ] conflict with any other inline `as` (including nested in `if/loop/parallel`)
- [ ] Keep reserved-name validation for inline `as` when provided.
- [ ] Update inline participant collector to include only named inline participants; anonymous inline must never be inserted into participants map.
- [ ] Ensure duplicate inline name is a validation error (not silent overwrite).

### Exit Criteria
- Anonymous inline participants parse and validate.
- Duplicate inline names fail validation with clear path/message.

---

## Phase 3 — Runtime State and CEL Namespace Redesign

### Files
- `internal/cel/variables.go`
- `internal/cel/env.go`
- `internal/cel/cel_test.go`
- `internal/engine/state.go`

### Tasks
- [ ] Extend runtime state to separate workflow-level and participant-level I/O.
- [ ] Introduce explicit fields (suggested names):
  - [ ] `WorkflowInputs map[string]any`
  - [ ] `WorkflowOutput any`
  - [ ] `CurrentInput any`
  - [ ] `CurrentOutput any`
- [ ] Keep `Steps`, `Execution`, `Loop`, `EventPayload`, `Now` behavior unchanged.
- [ ] Update CEL env declarations:
  - [ ] `input` as dynamic (`dyn`) rather than fixed map
  - [ ] `output` as dynamic (`dyn`)
  - [ ] `workflow` binding includes `inputs` and `output` fields
- [ ] Update `Bindings`:
  - [ ] `workflow.inputs` from `State.WorkflowInputs`
  - [ ] `workflow.output` from `State.WorkflowOutput`
  - [ ] `input` from `State.CurrentInput`
  - [ ] `output` from `State.CurrentOutput`

### Test updates
- [ ] Replace workflow-input CEL examples from `input.*` to `workflow.inputs.*`.
- [ ] Add CEL tests confirming participant-scoped `input` can be scalar or map.
- [ ] Add CEL tests confirming `output` variable exists in bindings.

### Exit Criteria
- CEL compile/eval supports `workflow.inputs.*` and scoped `input`/`output`.

---

## Phase 4 — Implicit I/O Chain Core in Engine

### Files
- `internal/engine/sequential.go`
- `internal/engine/engine.go`
- `internal/engine/engine_test.go`

### Tasks
- [ ] Refactor sequential execution to propagate a chain value between steps.
- [ ] Change internal execution signatures to carry incoming/outgoing chain.
- [ ] Update `engine.Run` fallback output behavior:
  - [ ] If top-level `output` is absent, return final chain value (not last named step output).
- [ ] Ensure chain is preserved for no-op control steps (`wait`) and skipped steps.

### Merge logic (must match spec v0.3)
When participant has both chain input and explicit input mapping:
- [ ] map + map: merge keys, explicit mapping wins on key conflict
- [ ] string + string: explicit string wins
- [ ] incompatible types: runtime error

### Suggested helper
Implement a focused helper in engine, e.g.:
- `mergeChainedInput(chain any, explicit any) (any, error)`

### Exit Criteria
- Sequential chain behavior works end-to-end.
- Merge conflict/incompatibility errors are explicit and deterministic.

---

## Phase 5 — Participant Execution Context Scoping

### Files
- `internal/engine/sequential.go`
- `internal/engine/control.go`
- `internal/engine/wait.go`

### Tasks
- [ ] Before evaluating `when` and participant input expressions, set `State.CurrentInput` to incoming chain (or merged value as appropriate by evaluation stage).
- [ ] Before participant execution, set `State.CurrentInput` to final resolved participant input.
- [ ] After participant success, set `State.CurrentOutput` to participant output.
- [ ] Ensure expression evaluation during participant lifecycle sees correct scoped `input`/`output`.
- [ ] Ensure non-participant control expressions still evaluate deterministically (with current chain bound to `input`, and `output` typically `nil`).

### Exit Criteria
- `input` inside participant expressions refers to current participant input, not workflow inputs.

---

## Phase 6 — Anonymous Inline Participant Runtime Support

### Files
- `internal/engine/sequential.go`
- `internal/participant/build.go`
- `internal/participant/build_test.go`
- `internal/engine/engine_test.go`
- `internal/integration/extra_test.go`

### Tasks
- [ ] Remove runtime hard-failure for inline participant missing `as`.
- [ ] Implement execution path for anonymous inline participants without requiring participant registry key.
- [ ] Keep named inline behavior unchanged (record step results under `as`, addressable in CEL).
- [ ] For anonymous inline participants:
  - [ ] execute normally
  - [ ] contribute to chain output
  - [ ] do not create `<step>` named binding

### Suggested implementation approach
- Add an exported builder for direct participant definitions, e.g. `participant.BuildOne(def, env, runnerFn)`.
- Reuse same participant constructors used by registry, avoiding duplicated switch logic.

### Exit Criteria
- Anonymous inline steps run successfully in top-level and nested control flows.

---

## Phase 7 — Control Flow Chain Semantics (`if`, `loop`, `parallel`)

### Files
- `internal/engine/control.go`
- `internal/engine/engine_test.go`
- `internal/integration/integration_test.go`

### Tasks
- [ ] `if` semantics:
  - [ ] true branch => chain is last output of `then`
  - [ ] false branch with `else` => chain is last output of `else`
  - [ ] false branch without `else` => chain unchanged
- [ ] `loop` semantics:
  - [ ] chain entering iteration N is previous iteration result
  - [ ] chain after loop is result of last step in last iteration
- [ ] `parallel` semantics:
  - [ ] each branch starts with same incoming chain
  - [ ] each branch computes independent final output
  - [ ] chain after parallel is ordered array of branch outputs
- [ ] Ensure parallel output ordering follows declaration order exactly.

### Exit Criteria
- Chain behavior for all control constructs matches §5.7.1 of spec.

---

## Phase 8 — Validation Rules for Chain Compatibility

### Files
- `internal/parser/validate.go`
- `internal/parser/validate_test.go`

### Tasks
- [ ] Add best-effort semantic checks for obviously incompatible explicit input patterns where determinable statically.
- [ ] Keep runtime validation as source of truth for dynamic CEL cases.
- [ ] Improve error messages to mention chain merge incompatibility clearly.

### Notes
Static inference of CEL result types is limited in current architecture. Do not block valid workflows based on uncertain inference.

### Exit Criteria
- Parser catches deterministic invalid cases.
- Runtime catches dynamic incompatibilities with clear errors.

---

## Phase 9 — Tests, Examples, and Documentation

### Files
- `internal/cel/cel_test.go`
- `internal/parser/*_test.go`
- `internal/engine/*_test.go`
- `internal/integration/*_test.go`
- `examples/*.flow.yaml`
- `README.md`
- `docs/HISTORY.md`

### Required new tests
- [ ] Anonymous inline minimal workflow (`flow: [{type: exec, run: ...}]`).
- [ ] Named inline uniqueness collisions.
- [ ] Empty `flow` is rejected at parse/lint time.
- [ ] Chain map merge precedence (explicit keys win).
- [ ] Chain string override (explicit string wins).
- [ ] Chain incompatible types error.
- [ ] `if` pass-through when false and no `else`.
- [ ] `parallel` returns ordered output array.
- [ ] Participant-scoped `input` vs `workflow.inputs` distinction.
- [ ] Inline sub-workflow step (`type: workflow` in `flow`) executes and exposes `<step>.output`.
- [ ] `emit` acknowledged timeout behavior (`ack: true` + `timeout` + `onTimeout`) is covered by tests, or is explicitly asserted as deferred/stubbed.

### Example/docs updates
- [ ] Update examples that currently rely on old workflow input namespace.
- [ ] Add at least one example using anonymous inline chain.
- [ ] Update README variable namespace and chain semantics for v0.3.
- [ ] Add HISTORY entry summarizing v0.3 upgrade decisions.

### Exit Criteria
- `go test ./...` passes.
- README/examples reflect v0.3 behavior.

---

## Suggested Commit Strategy (for small-agent implementation)

1. `schema: sync to spec v0.3`
2. `parser: allow anonymous inline + enforce inline name uniqueness`
3. `cel/state: introduce workflow.inputs + participant-scoped input/output bindings`
4. `engine: add implicit chain + merge rules`
5. `engine/control: chain semantics for if/loop/parallel`
6. `engine/participant: runtime support for anonymous inline execution`
7. `tests: cover v0.3 semantics`
8. `docs/examples: update for v0.3`

This order isolates risk and keeps each PR reviewable.

---

## Phase 10 — Spec-Text Alignment Follow-ups

### Files
- `internal/model/participant.go`
- `internal/participant/emit.go`
- `internal/parser/*_test.go`
- `internal/integration/*_test.go`
- `README.md`
- `docs/HISTORY.md`

### Tasks
- [ ] Decide and document handling for `emit.onTimeout` in v0.3:
  - [ ] implement participant-level `onTimeout` for `emit` acknowledged mode, or
  - [ ] explicitly defer (with rationale) until event-hub ack is no longer stubbed.
- [ ] If implemented:
  - [ ] add `OnTimeout` field to participant model and schema sync expectations.
  - [ ] apply timeout outcome in emit execution path (`fail` default, `skip` optional).
  - [ ] add unit/integration tests for ack-timeout behavior.
- [ ] If deferred:
  - [ ] document the temporary spec/runtime mismatch in `README.md` and `docs/HISTORY.md`.
  - [ ] add a tracked TODO item for v0.3.x follow-up.

### Exit Criteria
- `emit` acknowledged timeout behavior is either implemented and tested, or intentionally deferred with explicit project documentation.

---

## Final Definition of Done

The upgrade is complete only when all items are true:

1. Local schema and parser URL point to v0.3.
2. Anonymous inline participants work in all flow contexts.
3. Named inline collisions are rejected.
4. CEL namespace semantics match v0.3:
   - `workflow.inputs.*` for workflow inputs
   - `workflow.output` available
   - `input`/`output` are participant-scoped
5. Implicit chain works across sequential and control flow with correct merge behavior.
6. Default workflow return value (no `output:` block) is final chain output.
7. `flow` empty-array workflows are rejected with clear validation feedback.
8. Inline sub-workflow mode is covered by parser/integration tests.
9. `emit` acknowledged-timeout behavior is implemented and tested, or explicitly deferred in project docs with rationale.
10. Full test suite passes and docs/examples reflect new semantics.
