# duckflux — Runner Stack Decision

## The Question

What is the best stack for building a cross-platform runner for the duckflux workflow DSL?

## Candidates

### Go

Single static binary, no runtime dependencies. Official Google CEL implementation (`google/cel-go`) — the reference library. Native concurrency with goroutines and channels. The de facto language for workflow and infrastructure tooling (Argo, Temporal, Docker, Kubernetes, Terraform).

### Rust

Single binary with superior performance. However, CEL has no official implementation — only community `cel-rust`, which is less mature. The workflow tooling ecosystem in Rust is small. Development time is significantly higher for a performance gain the runner doesn't need (the bottleneck is participant I/O, not the engine).

### TypeScript / Node

Largest contributor ecosystem, fast prototyping. But cross-platform distribution is painful (requires Node installed, or bundling with pkg/bun). CEL has `cel-js` (community, incomplete). The advantage would only apply if the primary audience is JS/TS developers.

## Decision: Go

Go is the clear choice. The reasoning follows.

## Why Go

### 1. Official CEL Implementation

`google/cel-go` is the reference implementation of the Common Expression Language. It is maintained by Google, used in production by Kubernetes (admission policies), Firebase (security rules), and Envoy (RBAC). No other language has an equivalent — Rust and JavaScript only have community ports with varying completeness.

For a DSL that chose CEL as its expression language, using the reference implementation eliminates an entire class of compatibility and correctness risks.

### 2. Single Binary, Zero Dependencies

`go build` produces a statically linked binary. No runtime, no VM, no package manager on the target machine. Cross-compilation is built into the toolchain:

```bash
GOOS=linux   GOARCH=amd64 go build -o duckflux-linux
GOOS=darwin  GOARCH=arm64 go build -o duckflux-macos
GOOS=windows GOARCH=amd64 go build -o duckflux.exe
```

A user downloads a binary and runs it. No `node_modules`, no `pip install`, no Docker required.

### 3. Native Concurrency

Go's goroutines and channels map directly to duckflux's `parallel:` construct. Each parallel branch runs in a goroutine, the engine waits on a `sync.WaitGroup` or channel, and `context.Context` propagates timeout and cancellation to all branches.

This isn't just convenient — it's the correct abstraction. Parallel steps with timeout and error propagation require cooperative cancellation, which Go's context system was designed for.

### 4. Process Execution

The `exec` participant type needs to spawn subprocesses (shell commands, scripts). Go's `os/exec` package provides first-class process management with stdin/stdout piping, environment variable injection, and process group control. Combined with `context.Context`, timeouts on subprocesses are trivial.

### 5. Ecosystem Fit

The libraries needed for a duckflux runner all exist and are mature in Go:

| Need | Library | Maturity |
|------|---------|----------|
| CEL evaluation | `google/cel-go` | Official, production (Google) |
| YAML parsing | `gopkg.in/yaml.v3` | Standard, widely used |
| JSON Schema validation | `santhosh-tekuri/jsonschema` | Draft 2020-12 compliant |
| HTTP client | `net/http` (stdlib) | Built-in |
| Process execution | `os/exec` (stdlib) | Built-in |
| CLI framework | `cobra` or `urfave/cli` | Standard |
| Structured logging | `slog` (stdlib, Go 1.21+) | Built-in |

### 6. Precedent

Virtually every tool in the workflow and infrastructure space is written in Go: Argo Workflows, Temporal (server + workers), Docker, Kubernetes, Terraform, Consul, Vault, Prometheus, Grafana Alloy, Traefik. This isn't coincidence — the language's strengths (static binary, concurrency, fast compilation, stdlib) align perfectly with infrastructure tooling requirements.

Building duckflux in Go means it fits naturally into the ecosystem where it will be used.

### 7. Compilation Speed

Go compiles fast — a full build of a medium project takes seconds, not minutes. This matters for development velocity and CI/CD. A `go test ./...` cycle is nearly instant compared to equivalent Rust compile times.

## Proposed Structure

```
duckflux/
├── cmd/duckflux/         # CLI entrypoint
│   └── main.go           #   run, lint, validate commands
├── pkg/
│   ├── model/            # Core types: Workflow, Participant, FlowStep, etc.
│   ├── parser/           # YAML → model, schema validation
│   ├── cel/              # CEL environment setup, variable injection, evaluation
│   ├── runner/           # Execution engine
│   │   ├── engine.go     #   Main loop: sequential, loop, parallel, if
│   │   ├── context.go    #   Execution context, variable scoping
│   │   └── timeout.go    #   Timeout propagation and precedence
│   └── participant/      # Participant interface + type implementations
│       ├── interface.go  #   type Participant interface { Execute(ctx, input) (output, error) }
│       ├── exec.go       #   Shell command execution
│       ├── http.go       #   HTTP request
│       ├── mcp.go        #   MCP server delegation
│       └── workflow.go   #   Sub-workflow composition
├── schema/
│   └── duckflux.schema.json
└── examples/
    ├── minimal.flow.yaml
    └── code-review.flow.yaml
```

## What Go Does Not Solve

Go is the right choice for the runner, but some concerns are orthogonal to the language:

- **Plugin system for custom participant types** — Go's plugin system (`plugin` package) is Linux-only. Alternatives: gRPC-based plugin model (like Terraform/Vault), or WASM plugins.
- **LSP / Language Server** — Could be Go or TypeScript. VS Code extensions are typically TypeScript, but the LSP protocol is language-agnostic. The JSON Schema covers v1 editor support without an LSP.
- **GUI / Visual Editor** — If ever needed, this would be a separate web app (React/TypeScript), not part of the Go runner.

---

*Decision made: March 2026*
