# duckflux runner

Cross-platform CLI runner for the [duckflux workflow DSL](https://github.com/duckflux/spec) — a minimal, deterministic, runtime-agnostic language for orchestrating workflows through declarative YAML.

Define **what** happens and in **what order**. The runner decides **how**.

```yaml
participants:
  build:
    type: exec
    run: npm run build

  test:
    type: exec
    run: npm test

flow:
  - build
  - test
```

```bash
duckflux run ci.flow.yaml
```

## Requirements

- Go 1.24+

## Installation

### From source

```bash
git clone https://github.com/duckflux/runner.git
cd runner
make build
```

The binary is produced at `bin/duckflux`. Add it to your `PATH` or run it directly:

```bash
./bin/duckflux version
```

### Cross-compilation

```bash
GOOS=linux   GOARCH=amd64 go build -o duckflux-linux   ./cmd/duckflux
GOOS=darwin  GOARCH=arm64 go build -o duckflux-macos   ./cmd/duckflux
GOOS=windows GOARCH=amd64 go build -o duckflux.exe     ./cmd/duckflux
```

## Quick Start

Create a file called `hello.flow.yaml`:

```yaml
id: hello
name: Hello World
version: "1"

participants:
  greet:
    type: exec
    run: echo "Hello, duckflux!"

flow:
  - greet
```

Run it:

```bash
duckflux run hello.flow.yaml
```

Output:

```
Hello, duckflux!
```

## Command Reference

### `duckflux run`

Parse, validate, and execute a workflow.

```bash
duckflux run <file.flow.yaml> [flags]
```

| Flag | Description |
|------|-------------|
| `--input key=value` | Pass an input value (repeatable) |
| `--input-file path.json` | Load inputs from a JSON file |
| `--cwd path` | Base working directory for `exec` participants |
| `--verbose` | Enable debug logging |
| `--quiet` | Suppress all output except errors |

Input resolution priority (highest wins): `--input` flags > `--input-file` > stdin (piped JSON).

```bash
# With inline inputs
duckflux run deploy.flow.yaml --input branch=main --input env=staging

# With a JSON file
duckflux run deploy.flow.yaml --input-file inputs.json

# With piped JSON via stdin
echo '{"branch": "main"}' | duckflux run deploy.flow.yaml
```

### `duckflux lint`

Parse and validate a workflow without executing it. Checks JSON Schema conformance and semantic correctness (cross-references, reserved names, CEL expression syntax).

```bash
duckflux lint <file.flow.yaml>
```

Exits `0` and prints `OK` on success, exits `1` with errors otherwise.

### `duckflux validate`

Everything `lint` does, plus validates provided inputs against the workflow's declared input schema (required fields, types, formats).

```bash
duckflux validate <file.flow.yaml> [flags]
```

| Flag | Description |
|------|-------------|
| `--input key=value` | Input value to validate (repeatable) |
| `--input-file path.json` | JSON file with input values to validate |

```bash
duckflux validate deploy.flow.yaml --input branch=main --input max_retries=3
```

### `duckflux version`

Print the current version.

```bash
duckflux version
```

## Workflow Concepts

For the full specification, see the [duckflux spec](https://github.com/duckflux/spec).

### Participants

Participants are named steps that can be referenced in the flow. Each has a `type`:

| Type | Description | Status |
|------|-------------|--------|
| `exec` | Shell command execution | ✅ Implemented |
| `http` | HTTP request | ✅ Implemented |
| `human` | Interactive prompt (stdin/stdout) | ✅ Implemented |
| `workflow` | Sub-workflow composition | ✅ Implemented |
| `agent` | LLM-powered autonomous agent | 🔜 Stub (v2) |
| `mcp` | MCP server delegation | 🔜 Stub (v2) |
| `hook` | External event gateway | 🔜 Stub (v2) |

### Flow Control

| Construct | Description |
|-----------|-------------|
| Sequential | Steps run top-to-bottom |
| `loop` | Repeat steps until a condition is met or N times |
| `parallel` | Run steps concurrently |
| `if/then/else` | Conditional branching |
| `when` | Guard condition on a single step |

### Error Handling

Configurable per participant or per flow step invocation (flow overrides participant):

| `onError` | Behavior |
|-----------|----------|
| `fail` | Stop the workflow (default) |
| `skip` | Mark as skipped, continue |
| `retry` | Re-execute with backoff |
| `<participant>` | Redirect to a fallback participant |

### Timeouts

Resolution chain: **flow override > participant > defaults > none**.

### Working Directory (`exec`)

`exec` commands run with this precedence:
**participant.cwd > defaults.cwd > --cwd > current process cwd**.

### Expressions

All conditions and input mappings use [Google CEL](https://cel.dev). Expressions are type-checked at parse time and sandboxed (no I/O, no side effects).

## Examples

Example workflows are in the [`examples/`](examples/) directory.

### Minimal

A single-step workflow ([`examples/minimal.flow.yaml`](examples/minimal.flow.yaml)):

```yaml
id: minimal
name: Minimal Workflow
version: "1"

participants:
  greet:
    type: exec
    run: echo "Hello, duckflux!"

flow:
  - greet
```

```bash
duckflux run examples/minimal.flow.yaml
```

### Loop

Fixed iteration loop ([`examples/loop.flow.yaml`](examples/loop.flow.yaml)):

```yaml
id: loop-example
name: Loop Workflow
version: "1"

inputs:
  max_rounds:
    type: integer
    default: 3
    description: "Maximum number of loop iterations"

participants:
  counter:
    type: exec
    run: echo "running iteration"
    timeout: 5s

  check:
    type: exec
    run: echo "checking progress"
    timeout: 5s

flow:
  - loop:
      max: 3
      steps:
        - counter
        - check
```

```bash
duckflux run examples/loop.flow.yaml
```

### Parallel

Concurrent execution with a sequential follow-up ([`examples/parallel.flow.yaml`](examples/parallel.flow.yaml)):

```yaml
id: parallel-example
name: Parallel Workflow
version: "1"

participants:
  lint:
    type: exec
    run: echo "linting..."
    timeout: 30s

  test:
    type: exec
    run: echo "testing..."
    timeout: 30s

  build:
    type: exec
    run: echo "building..."
    timeout: 30s

  report:
    type: exec
    run: echo "all checks done"

flow:
  - parallel:
      - lint
      - test
      - build
  - report
```

```bash
duckflux run examples/parallel.flow.yaml
```

### Code Review Pipeline

A full pipeline with loops, conditionals, parallel steps, error handling, and output mapping ([`examples/code-review.flow.yaml`](examples/code-review.flow.yaml)):

```yaml
id: code-review
name: Code Review Pipeline
version: "1"

defaults:
  timeout: 5m

inputs:
  branch:
    type: string
    default: "main"
    description: "Branch to review"
  max_rounds:
    type: integer
    default: 3
    description: "Maximum review iterations"

participants:
  coder:
    type: exec
    run: echo '{"status":"coded","branch":"'"${BRANCH:-main}"'"}'
    timeout: 30s
    onError: retry
    retry:
      max: 2
      backoff: 1s

  reviewer:
    type: exec
    run: |
      echo '{"approved":true,"score":8,"comments":"Looks good"}'
    timeout: 30s
    onError: fail

  tests:
    type: exec
    run: echo "tests passed"
    timeout: 30s
    onError: skip

  lint:
    type: exec
    run: echo "lint passed"
    timeout: 30s
    onError: skip

  notify_success:
    type: http
    url: http://localhost:0
    method: POST
    timeout: 10s
    onError: skip

  notify_failure:
    type: http
    url: http://localhost:0
    method: POST
    timeout: 10s
    onError: skip

flow:
  - coder

  - loop:
      until: reviewer.output.approved == true
      max: 3
      steps:
        - reviewer
        - coder:
            when: reviewer.output.approved == false

  - parallel:
      - tests
      - lint

  - if: 'tests.status == "success" && lint.status == "success"'
    then:
      - notify_success
    else:
      - notify_failure

output:
  approved: reviewer.output.approved
  score: reviewer.output.score
  testResult: tests.status
  lintResult: lint.status
```

```bash
duckflux run examples/code-review.flow.yaml --input branch=develop
```

## Editor Support

Add the JSON Schema to your VS Code settings for autocomplete and validation in `.flow.yaml` files:

```json
{
  "yaml.schemas": {
    "./schema/duckflux.schema.json": "*.flow.yaml"
  }
}
```

Requires the [YAML extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml).

## Documentation

- [duckflux spec](https://github.com/duckflux/spec) — Full DSL specification
- [`docs/MOTIVATION.md`](docs/MOTIVATION.md) — Why Go was chosen as the runner stack
- [`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md) — Architecture and implementation phases
- [`docs/HISTORY.md`](docs/HISTORY.md) — Past decisions and changelog
