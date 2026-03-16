# duckflux Runner — Implementation Plan

Reference: [duckflux spec v0.1](https://github.com/duckflux/spec) · [MOTIVATION.md](./MOTIVATION.md)

---

## Architecture

```
duckflux/
├── cmd/duckflux/             # CLI entrypoint
│   └── main.go               #   cobra root + subcommands
├── internal/
│   ├── model/                # Core types mirroring the spec schema
│   │   ├── workflow.go       #   Workflow, Defaults, Inputs, Output
│   │   ├── participant.go    #   Participant, ParticipantType enum
│   │   ├── flow.go           #   FlowStep, LoopStep, ParallelStep, IfStep, OverrideStep
│   │   └── common.go         #   Duration, RetryConfig, OnError, CELExpression
│   ├── parser/               # YAML → model, structural validation
│   │   ├── parser.go         #   Parse(reader) → *model.Workflow
│   │   ├── schema.go         #   JSON Schema validation (embeds duckflux.schema.json)
│   │   └── validate.go       #   Semantic validation (cross-refs, reserved names, CEL parse)
│   ├── cel/                  # CEL environment and evaluation
│   │   ├── env.go            #   Build CEL environment with runtime variable declarations
│   │   ├── eval.go           #   Evaluate(expr, vars) → value
│   │   └── variables.go      #   Runtime variable bindings (workflow, execution, input, env, step, loop)
│   ├── engine/               # Execution engine
│   │   ├── engine.go         #   Run(ctx, workflow, inputs) → output
│   │   ├── state.go          #   Execution state: step results, variable store
│   │   ├── sequential.go     #   Step-by-step execution
│   │   ├── control.go        #   Loop, parallel, if/then/else, when guards
│   │   ├── timeout.go        #   Timeout resolution (flow > participant > defaults)
│   │   └── errors.go         #   Error handling: fail, skip, retry, redirect
│   └── participant/          # Participant interface + implementations
│       ├── registry.go       #   type Participant interface { Execute(ctx, input) (output, error) }
│       ├── exec.go           #   Shell command execution via os/exec
│       ├── http.go           #   HTTP requests via net/http
│       ├── emit.go           #   Event emission (stub/log in v1)
│       ├── workflow.go       #   Sub-workflow: recursive Parse + Run
│       └── mcp.go            #   Stub — returns error "mcp not yet supported"
├── schema/
│   └── duckflux.schema.json  # Embedded copy from spec repo
├── examples/
│   ├── minimal.flow.yaml
│   ├── loop.flow.yaml
│   ├── parallel.flow.yaml
│   └── code-review.flow.yaml
├── go.mod
└── go.sum
```

Use `internal/` (not `pkg/`) — nothing in this binary should be importable by other Go modules in v1.

---

## Key Design Decisions

### CLI Framework

Use **cobra** (`github.com/spf13/cobra`). It is the Go CLI standard (used by kubectl, Hugo, GitHub CLI). Three subcommands for v1:

```
duckflux run   <file.flow.yaml>  [--input key=value ...] [--env-file .env]
duckflux lint  <file.flow.yaml>
duckflux validate <file.flow.yaml> [--input key=value ...]
```

- `run` — parse, validate, execute, print output to stdout.
- `lint` — parse + structural validation + CEL parse check. No execution. Exit 0/1.
- `validate` — lint + input schema validation against provided inputs. No execution.

Input flags: `--input key=value` (repeatable), `--input-file inputs.json`, stdin pipe detection.
Output: last step output (or explicit `output:` mapping) to stdout. Structured JSON when output is a map.

### Execution State Model

A single `State` struct holds all runtime data. Every CEL expression resolves against this state.

```go
type State struct {
    Workflow   WorkflowMeta          // workflow.id, workflow.name, workflow.version
    Execution  ExecutionMeta         // execution.id, execution.number, execution.startedAt, execution.status, execution.context
    Input      map[string]any        // input.*
    Env        map[string]string     // env.*
    Steps      map[string]*StepResult // <step>.output, <step>.status, etc.
    Loop       *LoopContext          // loop.index, loop.iteration, loop.first, loop.last (nil outside loops)
}
```

Steps map is keyed by participant name. On re-execution (loop), the entry is overwritten — always latest result, per spec.

### CEL Environment

Build one `cel.Env` per workflow (at parse time), declaring all variables:
- `workflow` as an object type
- `execution` as an object type
- `input` fields from workflow inputs
- `env` as `map(string, string)`
- Each participant name as an object type (with `.output`, `.status`, `.retries`, etc.)
- `loop` as an object type (only within loop scope; re-declared per loop block)

Pre-compile all CEL expressions at parse/lint time. Runtime evaluation only calls `Program.Eval()` with the current state bindings.

### Timeout Resolution

Resolve effective timeout for each step invocation:
```
flow override > participant timeout > defaults.timeout > no timeout (context.Background)
```

Implementation: `context.WithTimeout(parentCtx, resolvedTimeout)` passed to participant `Execute()`. The participant must respect context cancellation.

### Error Handling Pipeline

On step failure, the engine executes this decision tree:
1. Resolve `onError` (flow override > participant > default `"fail"`)
2. `"fail"` → wrap error, abort workflow
3. `"skip"` → mark step as `skipped`, continue
4. `"retry"` → re-execute up to `retry.max` times with backoff; if all retries fail, treat as `"fail"`
5. `<participant name>` → execute the named participant as fallback; if fallback fails, treat as `"fail"`

### Parallel Execution

`parallel:` maps to goroutines + `sync.WaitGroup`. Each branch gets a child `context.Context` derived from the parent (inheriting timeout/cancellation). Results are collected into the shared `State.Steps` map with a mutex. If any branch fails and its `onError` is `"fail"`, cancel all other branches via context.

### Sub-workflow Execution

`workflow` participant type: parse the referenced YAML file, create an isolated child `State` (fresh `execution.context`, mapped `input`), execute, and map the child's output as `<step>.output` in the parent state.

### Input/Output String-by-Default

When no schema is defined, all inputs/outputs are `string`. JSON auto-detection on participant output: attempt `json.Unmarshal` → if success, store as `map[string]any`; if fail, store as raw string. This enables field access (`step.output.field`) for JSON outputs without explicit schema.

---

## Phases

### Phase 0 — Project Bootstrap

Initialize the Go module, directory structure, CI pipeline, and dependency management.

- `go mod init github.com/duckflux/runner`
- Create the directory tree from the architecture above (empty files with package declarations)
- Add `Makefile` with targets: `build`, `test`, `lint`, `run`
- Set up GitHub Actions CI: `go vet`, `golangci-lint`, `go test ./...`
- Embed `duckflux.schema.json` from the spec repo into `schema/`
- Add cobra dependency, wire up `cmd/duckflux/main.go` with empty `run`, `lint`, `validate` subcommands

**No dependencies on other phases. Start here.**

### Phase 1 — Core Model Types

Translate the JSON Schema into Go structs. These types are the contract between parser, engine, and participants.

- Define all types in `internal/model/`: `Workflow`, `Participant`, `FlowStep` (union type), `LoopStep`, `ParallelStep`, `IfStep`, `ParticipantOverrideStep`, `RetryConfig`, `Defaults`, `InputField`, `WorkflowOutput`
- `FlowStep` is the central challenge — it's a union (string | loop | parallel | if | override). Implement via custom `UnmarshalYAML` that inspects the YAML node type and delegates
- `ParticipantType` as a string enum with validation
- `Duration` as a custom type wrapping `time.Duration` with YAML unmarshaling for `"30s"`, `"5m"` format
- Reserved names constant list: `workflow`, `execution`, `input`, `output`, `env`, `loop`
- Comprehensive unit tests for all YAML unmarshaling edge cases

**No dependencies. Can parallelize with Phase 2.**

### Phase 2 — CEL Environment & Evaluation

Build the CEL integration layer that all expressions depend on.

- Set up `cel.Env` factory that accepts a `*model.Workflow` and produces a configured environment
- Declare all runtime variable types (workflow, execution, input, env, step results, loop)
- Implement `Compile(expr string) → cel.Program` (used at lint time)
- Implement `Eval(program, state) → any` (used at runtime)
- Handle type coercion between CEL results and Go types
- Custom CEL functions if needed (none expected for v1 — standard library is sufficient)
- Unit tests: expression parsing, variable resolution, type errors at compile time

**No dependencies. Can parallelize with Phase 1.**

### Phase 3 — Parser & Validation

YAML parsing and multi-level validation.

- `Parse(reader) → *model.Workflow` using `gopkg.in/yaml.v3`
- JSON Schema validation: embed `duckflux.schema.json` via `go:embed`, validate raw YAML against schema using `santhosh-tekuri/jsonschema`
- Semantic validation (post-parse checks):
  - All flow step references exist in `participants`
  - `onError` redirect targets exist in `participants`
  - Reserved names not used as participant names
  - `loop` has at least `until` or `max`
  - All CEL expressions parse successfully (use Phase 2's `Compile`)
- Return structured validation errors with line numbers where possible
- Wire into `duckflux lint` CLI command

**Depends on: Phase 1 (model types). Partially depends on Phase 2 (CEL compile check).**

### Phase 4 — Execution Engine

The core runtime that walks the flow and executes steps.

- `State` struct with all runtime variables
- Sequential executor: iterate `flow` steps, resolve participant, call `Execute`, store result
- Control flow dispatch:
  - `loop`: evaluate `until`/`max`, iterate, update `loop.*` context, overwrite step results each iteration
  - `parallel`: spawn goroutines, collect results, handle cancellation on failure
  - `if`: evaluate condition, execute `then` or `else` branch
  - `when` guard: evaluate condition, skip if false
- Flow-level overrides: merge participant definition with flow-level overrides before execution
- Timeout resolution and `context.WithTimeout` wrapping
- Error handling pipeline (fail / skip / retry with backoff / redirect)
- Input mapping: evaluate CEL expressions in participant `input` to build the data passed to `Execute`
- Output mapping: evaluate workflow `output` expressions to build final result
- Wire into `duckflux run` CLI command

**Depends on: Phase 1 (model), Phase 2 (CEL), Phase 3 (parser).**

### Phase 5 — Participant Implementations

Implement each participant type behind the common interface:

```go
type Participant interface {
    Execute(ctx context.Context, input any) (any, error)
}
```

Build a registry that maps `model.ParticipantType` → constructor.

Implementations (can be built in parallel, all depend on the interface from Phase 4):

- **exec**: `os/exec.CommandContext`, pipe stdin from input, capture stdout/stderr, respect context cancellation. Environment variable injection.
- **http**: `net/http` client, build request from participant config (url, method, headers, body with CEL-evaluated values), return response body. Respect context timeout.
- **emit**: Publish an event payload to the runner event hub (v1 baseline may log/stub while preserving DSL contract).
- **workflow**: Parse sub-workflow YAML, create child state with mapped inputs, recursive `engine.Run`, return child output.
- **mcp**: Stub returning `"mcp participant type is not yet implemented"` error. Define the interface so v2 can plug in.
- **hook**: Not implemented in v1 per spec (depends on signals). Stub with error.

**Depends on: Phase 4 (engine interface, state). Individual participants can be built in parallel.**

### Phase 6 — CLI & Integration

Finalize the CLI and end-to-end behavior.

- `duckflux run`: parse → validate → resolve inputs (flags, file, stdin) → execute → print output
- `duckflux lint`: parse → schema validate → semantic validate → CEL compile check → exit 0/1
- `duckflux validate`: lint + input schema validation against provided `--input` values
- `--input key=value` flag (repeatable), `--input-file inputs.json`
- `--verbose` / `--quiet` flags for controlling log output
- Structured logging via `slog` (Go 1.21+) — log step start/end, duration, status
- Output formatting: raw string to stdout for string output, JSON for map output
- Version command: `duckflux version`
- Example workflow files in `examples/`
- End-to-end integration tests using `exec` and `http` participants with real commands

**Depends on: Phase 4, Phase 5. This is the final integration phase.**

---

## Dependency Graph

```
Phase 0 ─────────────────────────────────────────────┐
    │                                                 │
    ├──→ Phase 1 (Model) ──┐                          │
    │                      ├──→ Phase 3 (Parser) ──┐  │
    ├──→ Phase 2 (CEL) ────┘                       │  │
    │                                              │  │
    │              Phase 4 (Engine) ←──────────────┘  │
    │                   │                             │
    │                   ├──→ Phase 5a (exec)          │
    │                   ├──→ Phase 5b (http)          │
    │                   ├──→ Phase 5c (emit)          │
    │                   ├──→ Phase 5d (workflow)      │
    │                   └──→ Phase 5e (mcp stub)      │
    │                                                 │
    └──→ Phase 6 (CLI & Integration) ←───── all above │
```

Maximum parallelism: Phase 1 + Phase 2 in parallel, then Phase 5a–5e all in parallel.

---

## Dependencies (Go Modules)

| Module | Purpose | Version |
|--------|---------|---------|
| `github.com/spf13/cobra` | CLI framework | latest stable |
| `gopkg.in/yaml.v3` | YAML parsing | latest stable |
| `github.com/google/cel-go` | CEL expression engine | latest stable |
| `github.com/santhosh-tekuri/jsonschema/v6` | JSON Schema validation | latest stable |

All other functionality uses Go stdlib (`net/http`, `os/exec`, `context`, `slog`, `sync`, `embed`).

---

## GitHub Issues

Below are the issues to create in `duckflux/runner`. All should be labeled `v1`. Dependencies are listed per issue.

### Issue 1: Project Bootstrap — Go module, directory structure, CI, Makefile

**Phase 0.** Initialize the repository with Go module, create all directories and package stubs, add Makefile (build/test/lint), set up GitHub Actions CI, embed `duckflux.schema.json`, add cobra with empty subcommands.

Dependencies: none.

### Issue 2: Core Model Types — Go structs for the spec schema

**Phase 1.** Define `Workflow`, `Participant`, `FlowStep` (union), `LoopStep`, `ParallelStep`, `IfStep`, `OverrideStep`, `RetryConfig`, `Defaults`, `InputField`, `Duration`, `WorkflowOutput` in `internal/model/`. Implement custom YAML unmarshaling for union types. Include reserved name constants. Unit tests for all unmarshaling paths.

Dependencies: #1.

### Issue 3: CEL Environment & Expression Evaluation

**Phase 2.** Build CEL environment factory from workflow definition. Declare all runtime variable types. Implement `Compile` (parse-time) and `Eval` (runtime). Unit tests for variable resolution, type checking, and standard CEL functions.

Dependencies: #1.

### Issue 4: YAML Parser & JSON Schema Validation

**Phase 3a.** Implement `Parse(reader) → *model.Workflow` with `yaml.v3`. Embed and validate against `duckflux.schema.json`. Return structured errors.

Dependencies: #2.

### Issue 5: Semantic Validation & Lint Command

**Phase 3b.** Post-parse validation: cross-references (flow → participants, onError targets), reserved names, loop constraints, CEL expression compilation. Wire into `duckflux lint` CLI command.

Dependencies: #2, #3, #4.

### Issue 6: Execution Engine — State, Sequential Execution, Input/Output Mapping

**Phase 4a.** Implement `State` struct, sequential step execution, input mapping (CEL evaluation), output mapping (workflow output resolution), string-by-default with JSON auto-detection.

Dependencies: #2, #3, #4.

### Issue 7: Execution Engine — Control Flow (loop, parallel, if, when)

**Phase 4b.** Implement loop (until/max, loop context variables), parallel (goroutines, WaitGroup, context cancellation), if/then/else branching, when guards.

Dependencies: #6.

### Issue 8: Timeout System

**Phase 4c.** Implement timeout resolution (flow > participant > defaults > none). Wrap each step execution with `context.WithTimeout`. Handle timeout as failure routed through onError.

Dependencies: #6.

### Issue 9: Error Handling & Retry

**Phase 4d.** Implement the onError pipeline: fail (abort), skip (continue), retry (with max/backoff/factor), redirect (execute fallback participant). Exponential backoff calculation.

Dependencies: #6.

### Issue 10: Participant — exec (shell command execution)

**Phase 5a.** Implement `exec` participant: `os/exec.CommandContext`, stdin piping, stdout/stderr capture, environment variable injection, context cancellation.

Dependencies: #6.

### Issue 11: Participant — http (HTTP requests)

**Phase 5b.** Implement `http` participant: build request from config (url, method, headers, body), execute via `net/http`, return response body, respect context timeout.

Dependencies: #6.

### Issue 12: Participant — emit (event publication)

**Phase 5c.** Implement `emit` participant: publish/log event payload, preserve event metadata, and keep runtime behavior deterministic in v1.

Dependencies: #6.

### Issue 13: Participant — workflow (sub-workflow composition)

**Phase 5d.** Implement `workflow` participant: parse referenced YAML, create isolated child state, recursive engine execution, map child output to parent.

Dependencies: #6, #4.

### Issue 14: Participant stubs — mcp, hook

**Phase 5e.** Stub implementations for `mcp` and `hook` participant types. Return clear "not yet implemented" errors. Define interfaces for future v2 implementation.

Dependencies: #6.

### Issue 15: CLI — run command, input handling, output formatting

**Phase 6a.** Implement `duckflux run`: parse → validate → resolve inputs (`--input`, `--input-file`, stdin) → execute → print output. Add `--verbose`/`--quiet` flags. Structured logging via `slog`. JSON output for maps, raw string otherwise. `duckflux version` command.

Dependencies: #5, #6, #7, #8, #9, #10, #11, #12, #13, #14.

### Issue 16: CLI — validate command

**Phase 6b.** Implement `duckflux validate`: lint + input schema validation against provided `--input` values. Validate required fields, types, constraints.

Dependencies: #5.

### Issue 17: Example Workflows & Integration Tests

**Phase 6c.** Create example `.flow.yaml` files (minimal, loop, parallel, code-review from spec). End-to-end integration tests exercising `exec` and `http` participants with real commands and a local test HTTP server.

Dependencies: #15.
