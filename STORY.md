# duckflux Runner — Upgrade to spec v0.3

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

## Current Gap Summary

|   |
| - |

## Implementation Principles

1. Keep existing architecture (`model`, `parser`, `cel`, `engine`, `participant`) and avoid broad package reshuffles.
2. Ship in small phases with `go test ./...` at each phase.
3. Prefer explicit runtime context over hidden global behavior for CEL scope.
4. For ambiguous spec corners, use deterministic defaults and document them.

### Assumptions to unblock implementation

1. If a step is skipped (`when: false` or `onError: skip`), the implicit chain remains unchanged.
2. `wait` step does not create output; it preserves incoming chain unchanged.
3. `workflow.output` is `null` during execution and is set only after final output resolution.
4. Static merge compatibility checks are best-effort; runtime merge checks are authoritative.

## Suggested Commit Strategy

1. `schema: sync to spec v0.3`
2. `parser: allow anonymous inline + enforce inline name uniqueness`
3. `cel/state: introduce workflow.inputs + participant-scoped input/output bindings`
4. `engine: add implicit chain + merge rules`
5. `engine/control: chain semantics for if/loop/parallel`
6. `engine/participant: runtime support for anonymous inline execution`
7. `tests: cover v0.3 semantics`
8. `docs/examples: update for v0.3`

This order isolates risk and keeps each PR reviewable.

## Final Definition of Done

1. Local schema and parser URL point to v0.3.
2. Anonymous inline participants work in all flow contexts.
3. Named inline collisions are rejected.
4. CEL namespace semantics match v0.3: `workflow.inputs.*` for workflow inputs, `workflow.output` available, `input`/`output` are participant-scoped.
5. Implicit chain works across sequential and control flow with correct merge behavior.
6. Default workflow return value (no `output:` block) is final chain output.
7. `flow` empty-array workflows are rejected with clear validation feedback.
8. Inline sub-workflow mode is covered by parser/integration tests.
9. `emit` acknowledged-timeout behavior is implemented and tested, or explicitly deferred in project docs with rationale.
10. Full test suite passes and docs/examples reflect new semantics.
