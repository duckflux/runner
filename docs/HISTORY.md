# History

## Past Decisions

### Spec v0.3 upgrade
**Date:** March 2026 · **By:** @ggondim, @copilot

Upgraded the runner to match `duckflux` spec v0.3. Notable changes:

- **Variable namespace redesign**: Workflow inputs moved from `input.*` to `workflow.inputs.*`. `input` and `output` are now participant-scoped (bound to the current step's I/O during execution). `workflow.output` is available for reading the final resolved output.
- **Implicit I/O chain**: Output of step N automatically becomes input of step N+1. Chain propagates through all control constructs (`if`, `loop`, `parallel`). When no explicit `output:` block is defined, the workflow returns the final chain value.
- **Chain merge rules**: When a step has both chain input and explicit `input:` mapping: map+map merges keys (explicit wins on conflict); for all other type combinations, explicit wins.
- **Anonymous inline participants**: Flow steps with `type` but no `as` field are now valid. They execute normally and contribute to the chain without creating a named binding.
- **Named inline uniqueness**: `as` values on inline participants must be globally unique across all top-level participant names and other inline `as` values. Duplicates are rejected at parse time.
- **Parallel chain output**: After a `parallel` block, the chain value is an ordered array of each branch's final output (declaration order).
- **Schema sync**: Embedded JSON Schema updated to v0.3; `method` field now uses enum constraint; `inlineParticipant` no longer requires `as`.
- **`flow` non-empty contract**: `flow: []` is rejected at both schema and semantic validation levels.
- **`emit` acknowledged-timeout**: Deferred — the event hub is currently stubbed (v1), so `emit.onTimeout` for `ack: true` mode is not yet implemented. Will be addressed when the event hub integration is built out (tracked as a v0.3.x follow-up).

Implementation preserves backward compatibility for workflows that don't use the new chain features. CEL type for `input` changed from `map[string]dyn` to `dyn` to support both map and scalar chain values.

---

### Spec v0.2 upgrade
**Date:** March 2026 · **By:** @ggondim, @copilot

Upgraded the runner to match `duckflux` spec v0.2. Notable changes:

- New JSON Schema (draft 2020-12) and relaxed required fields; `participants` is now optional and inline participants are supported.
- New `emit` participant type (v1: stubbed/logged). `mcp` moved to use `tool` field (replaces `operation`). `hook` removed from v0.2.
- Flow changes: `wait` step added (timeout/event/polling), `parallel` accepts full `FlowStep` entries (inline steps), `if` now uses `condition` field, and `loop` supports `as` for custom context names.
- CEL environment: added `event` and `now` variables, and `cwd` propagation from `exec` participants. The runner rewrites loop context names for CEL expressions.
- Engine: `runWait` implemented (sleep/poll/event stubs), inline participants executed inline, and step `cwd` is recorded in `StepResult`.

Implementation is backward-compatible where possible; several v1 features remain stubs (event hub integration, full `emit` ack flows, advanced MCP delegation) and are documented as TODOs.

### 1. Package architecture under `internal/` with clear separation of responsibilities
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **By:** @ggondim  
The project was structured as a Go module using `internal/` with five single-responsibility packages: `model`, `parser`, `cel`, `engine`, `participant`. The CLI uses Cobra with the subcommands `run`, `lint`, and `validate`. External dependencies are limited to: `cel-go`, `yaml.v3`, `jsonschema/v6`, `cobra`.

---

### 2. `FlowStep` as a union type with custom `UnmarshalYAML`
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **By:** @ggondim  
The `FlowStep` type is a union that dispatches to the correct concrete type (exec, http, loop, parallel, if, workflow, etc.) based on the YAML node format during unmarshalling. This keeps the model API clean without requiring explicit discriminator wrappers in users' YAML.

---

### 3. Single `State` as the CEL evaluation context
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **By:** @ggondim  
A single `State` struct serves as the CEL evaluation context for the entire workflow. Each step's results are indexed by participant name and overwritten on each re-execution within loops. This simplifies the state model at the cost of not preserving the history of previous iterations inside a loop.

---

### 4. Timeout resolution chain: `flow > participant > defaults > none`
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#26](https://github.com/duckflux/runner/pull/26) · **By:** @ggondim (decision), @copilot (implementation)  
Timeouts are resolved through a descending priority chain: flow step override > participant declaration > global defaults > no timeout. Timeout failures go through the same `onError` pipeline, since `context.DeadlineExceeded` manifests as a normal error from `Execute`.

---

### 5. `onError` pipeline: fail → skip → retry → redirect
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#27](https://github.com/duckflux/runner/pull/27) · **By:** @ggondim (decision), @copilot (implementation)  
The error handling pipeline follows the order: `fail` (default) → `skip` → `retry` (with exponential backoff and context cancellation) → `redirect` to a named participant. Retry implements exponential backoff while respecting context cancellation.

---

### 6. `parallel:` mapped to goroutines + `sync.WaitGroup` with failure cancellation
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#25](https://github.com/duckflux/runner/pull/25) · **By:** @ggondim (decision), @copilot (implementation)  
Parallel branches are executed as goroutines coordinated by a `sync.WaitGroup`. If any branch fails, the shared context is cancelled, interrupting the remaining in-progress branches. `State` has a `sync.RWMutex` for thread-safe writing of step results.

---

### 7. `mcp` and `hook` are stubs in v1
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#32](https://github.com/duckflux/runner/pull/32) · **By:** @ggondim  
The `mcp` and `hook` participant types were intentionally left as stubs that return `"not yet implemented"` in v1. Each stub defines a typed v2 interface to guide future implementations while maintaining JSON schema conformance.

---

### 8. Internal rewrite of `loop` → `_loop` to avoid conflict with a CEL reserved word
**PR:** [#21](https://github.com/duckflux/runner/pull/21) · **Decision:** @ggondim · **Implementation:** @copilot  
`loop` is a reserved identifier in CEL. Instead of exposing this implementation detail to workflow developers, the runner transparently rewrites `loop.` to `_loop.` before compiling any CEL expression. Expressions like `loop.index` and `loop.first` work naturally. The rewrite uses a regex with word-boundary to avoid affecting identifiers that merely *contain* "loop" (e.g. `myloop.field`).

---

### 9. Circular import problem in the `workflow` participant solved via dependency injection
**PR:** [#31](https://github.com/duckflux/runner/pull/31) · **By:** @copilot  
The `engine` package already imports `participant`, so `participant/workflow.go` cannot import `engine` (circular import). The solution was dependency injection via `SubWorkflowRunnerFunc`: a callback function provided by the wiring layer (CLI) that closes over `parser.Parse` and `engine.Run`, keeping the `participant` package completely free of `engine` imports.

---

### 10. JSON schema validation with an embedded schema via `embed`
**PR:** [#22](https://github.com/duckflux/runner/pull/22) · **By:** @copilot  
Schema validation uses `santhosh-tekuri/jsonschema/v6` with the `duckflux.schema.json` file embedded in the binary via `//go:embed`. The parse pipeline executes three sequential phases: JSON Schema → YAML decode → semantic checks.

---

### 11. Type coercion for CLI inputs: strings are parsed to the declared types
**PR:** [#34](https://github.com/duckflux/runner/pull/34) · **By:** @copilot  
`--input key=value` flags always arrive as strings. The input validator (`ValidateInputs`) performs type coercion: `"42"` is valid for `integer`, `"true"` for `boolean`, etc. Unknown formats pass through silently for future compatibility. When `--input-file` and `--input` conflict, the `--input` flag takes precedence.

---

### 12. Observability fields added to `StepResult`
**PR:** [#36](https://github.com/duckflux/runner/pull/36) · **By:** @copilot  
`StepResult` gained the fields `startedAt`, `finishedAt`, `duration`, and `error`, populated by the engine on each step execution. Structured logging via `slog` was added per step (start/end/duration/status).

---

### 13. Sub-workflow path resolved relative to the parent workflow
**PR:** [#36](https://github.com/duckflux/runner/pull/36) · **By:** @copilot  
The `path` field of a `workflow` participant is resolved relative to the directory of the invoking workflow, not the current working directory. This makes workflows portable and predictable regardless of where the binary is called from.

---

### 14. Non-boolean CEL expressions in `if`/`when`/`loop.until` are treated as errors
**PR:** [#36](https://github.com/duckflux/runner/pull/36) · **By:** @copilot  
If a CEL expression in a control-flow context (`if`, `when`, `loop.until`) produces a non-boolean result, the engine returns an explicit error instead of silently coercing it. This prevents hard-to-diagnose bugs caused by malformed expressions.

---

### 15. Phased build plan with a dependency graph optimized for parallelism
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **By:** @ggondim  
Development was planned in phases with explicit dependencies: Phase 0 (bootstrap) → Phase 1 `model` ∥ Phase 2 `cel` (independent, executable in parallel) → Phase 3 (parser) → Phase 4 (engine: sequential, control flow, timeout, error handling) → Phase 5a–f (participants, all parallel with each other) → Phase 6 (CLI, examples, tests and e2e).

---

### 16. CEL program cache + workflow precompilation for runtime reuse
**PR:** local (March 2026) · **By:** @copilot  
`cel.Environment` now caches compiled programs by rewritten expression (including `loop.` → `_loop.` translation). The engine precompiles all workflow CEL expressions at startup and runtime reuses the cached programs, reducing repeated compilation during step execution.

---

### 17. Dynamic HTTP field evaluation with CEL and literal fallback
**PR:** local (March 2026) · **By:** @copilot  
For `http` participants, `url`, `method`, `headers`, and `body` can now be resolved dynamically from CEL at runtime using the current state. To preserve backward compatibility with static workflows, string values that do not compile as CEL are treated as plain literals.

---

### 18. Workflow output accepts scalar CEL expression (string) or map
**PR:** local (March 2026) · **By:** @copilot  
The schema now matches the model behavior: `output` may be either a single CEL expression string (scalar output) or an object map of named CEL expressions. This removed a schema/model mismatch that previously rejected valid scalar `output` definitions during lint/parse.

---

### 19. Working directory support for `exec` with explicit precedence
**PR:** local (March 2026) · **By:** @copilot  
`cwd` support was added at three levels: CLI (`duckflux run --cwd`), workflow defaults (`defaults.cwd`), and participant (`exec.cwd`). The effective working directory follows the precedence: `participant.cwd` > `defaults.cwd` > `--cwd` > process cwd. `exec.cwd` and `defaults.cwd` support CEL expressions; relative paths are resolved against the selected base working directory; execution uses `cmd.Dir`.

---

## Changelog (resolved issues)

| Issue | Title |
|-------|--------|
| [#18](https://github.com/duckflux/runner/issues/18) | Example Workflows & Integration Tests |
| [#17](https://github.com/duckflux/runner/issues/17) | CLI — validate command |
| [#16](https://github.com/duckflux/runner/issues/16) | CLI — run command, input handling, output formatting |
| [#15](https://github.com/duckflux/runner/issues/15) | Participant stubs — mcp, hook |
| [#14](https://github.com/duckflux/runner/issues/14) | Participant — workflow (sub-workflow composition) |
| [#12](https://github.com/duckflux/runner/issues/12) | Participant — http (HTTP requests) |
| [#11](https://github.com/duckflux/runner/issues/11) | Participant — exec (shell command execution) |
| [#10](https://github.com/duckflux/runner/issues/10) | Error Handling & Retry |
| [#9](https://github.com/duckflux/runner/issues/9) | Timeout System |
| [#8](https://github.com/duckflux/runner/issues/8) | Execution Engine — Control Flow (loop, parallel, if, when) |
| [#7](https://github.com/duckflux/runner/issues/7) | Execution Engine — State, Sequential Execution, Input/Output Mapping |
| [#6](https://github.com/duckflux/runner/issues/6) | Semantic Validation & Lint Command |
| [#5](https://github.com/duckflux/runner/issues/5) | YAML Parser & JSON Schema Validation |
| [#4](https://github.com/duckflux/runner/issues/4) | CEL Environment & Expression Evaluation |
| [#3](https://github.com/duckflux/runner/issues/3) | Core Model Types — Go structs for the spec schema |
| [#2](https://github.com/duckflux/runner/issues/2) | Project Bootstrap — Go module, directory structure, CI, Makefile |
