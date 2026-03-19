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
| `--event-backend` | Event hub backend: `memory` (default), `nats`, or `redis` |
| `--nats-url` | NATS server URL (required when `--event-backend=nats`) |
| `--nats-stream` | JetStream stream name (default: `duckflux-events`) |
| `--redis-addr` | Redis server address (default: `localhost:6379`) |
| `--redis-db` | Redis database number (default: `0`) |
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
| `workflow` | Sub-workflow composition | ✅ Implemented |
| `emit` | Publish an event to the event hub | ✅ Implemented |
| `mcp` | MCP server delegation (`tool` field replaces `operation`) | 🔜 Stub (v2) |

### Flow Control

| Construct | Description |
|-----------|-------------|
| Sequential | Steps run top-to-bottom |
| `loop` | Repeat steps until a condition is met or N times |
| `parallel` | Run steps concurrently |
| `if/then/else` | Conditional branching |
| `when` | Guard condition on a single step |

Note (spec v0.3):

- **Implicit I/O chain**: The output of step N automatically becomes the input of step N+1. When no explicit `output:` is defined, the workflow returns the final chain value.
- **Variable namespaces**: Workflow-level inputs are accessed via `workflow.inputs.*`. Inside a participant step, `input` refers to the participant's scoped input (chain + explicit merge) and `output` refers to the participant's output.
- **Anonymous inline participants**: Flow steps can define `type` without `as` — they execute normally and contribute to the chain without a named binding.
- **Chain merge rules**: When a step has both chain input and explicit `input:`, maps are merged (explicit wins on key conflict); for other types, explicit always wins.
- **Parallel chain output**: After a `parallel` block, the chain value is an ordered array of each branch's final output.
- The `wait` construct is available to pause execution until an event, a timeout, or a polling condition is met.
- Inline participants are supported: a `flow` step can contain an inline participant definition instead of referencing the top-level `participants:` map. Named inline `as` values must be globally unique.
- `loop` supports the `as` field to rename the loop context (for example `as: attempt` exposes `attempt.index`). The runner rewrites the context for CEL expressions.
- `if` is now an object with a `condition` field: `if: { condition: "expr", then: [...], else: [...] }`.

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

### Inline participant example

Inline participants allow defining a participant directly inside the `flow` without adding it to the top-level `participants` map:

```yaml
flow:
  - myInlineStep:
    type: exec
    run: echo "inline participant"

# This runs the inline exec once and doesn't require a named participant entry
```

### Wait example

The `wait` step supports simple timeouts, event matching, and polling:

```yaml
- wait:
    timeout: 30s

# event-based:
- wait:
    event: order.created
    match: "event.orderId == 'ORD-001'"
    timeout: 10s
    onTimeout: fail
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

## Event Hub

The event hub connects `emit` participants to `wait.event` steps — including across parent and sub-workflows. Three backends are supported.

### Backends

| Backend | Description | Replay (subscribe-after-publish) | Fan-out |
|---------|-------------|----------------------------------|---------|
| `memory` | In-process Go channel (default) | ✅ Yes | ✅ Yes |
| `nats` | NATS JetStream | ❌ Ephemeral consumers start at latest offset | ✅ Yes (independent consumers) |
| `redis` | Redis Streams | ✅ Yes (consumer group reads from stream start) | ✅ Yes (separate consumer groups) |

### CLI Flags

```
--event-backend string   Event hub backend: memory, nats, or redis (default "memory")
--nats-url      string   NATS server URL, e.g. nats://localhost:4222 (required for --event-backend=nats)
--nats-stream   string   JetStream stream name (default "duckflux-events")
--redis-addr    string   Redis server address (default "localhost:6379")
--redis-db      int      Redis database number (default 0)
```

```bash
# Default: in-memory hub (no extra infrastructure needed)
duckflux run workflow.flow.yaml

# NATS JetStream
duckflux run workflow.flow.yaml --event-backend=nats --nats-url=nats://localhost:4222

# Redis Streams
duckflux run workflow.flow.yaml --event-backend=redis --redis-addr=localhost:6379
```

### `emit` Participant

Publishes an event to the hub. Other steps in the same workflow (or any sub-workflow) that have a matching `wait.event` will receive it.

```yaml
participants:
  notify:
    type: emit
    event: order.created
    payload:
      orderId: "ORD-001"
      total: 99.95

  notify_ack:
    type: emit
    event: payment.processed
    ack: true          # block until the broker confirms delivery
    timeout: 5s
    onTimeout: fail    # or: skip (continue without error on timeout)
    payload:
      transactionId: "TXN-42"
      status: "approved"
```

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | Event name / topic (dot-notation supported, e.g. `payment.processed`) |
| `payload` | any | Arbitrary data attached to the event; available as `event.*` in `wait.event` match expressions |
| `ack` | bool | If `true`, block until the broker acknowledges delivery (default: `false`) |
| `timeout` | duration | Maximum time to wait for acknowledgement when `ack: true` |
| `onTimeout` | `fail` \| `skip` | What to do when the ack deadline expires |

The participant's output is an object with a single `ack` field:

```yaml
output: notify_ack.output.ack   # true when ack succeeded, false on timeout+skip
```

### `wait.event` Step

Blocks the workflow until an event with the matching name arrives on the hub. An optional CEL `match` expression filters events by payload content.

```yaml
flow:
  - wait:
      event: order.created
      match: "event.orderId == 'ORD-001'"   # optional CEL filter
      timeout: 10s
      onTimeout: fail                        # or: skip
```

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | Event name to wait for |
| `match` | CEL expression | Optional filter evaluated against the event payload (`event.*`) |
| `timeout` | duration | How long to wait before invoking `onTimeout` |
| `onTimeout` | `fail` \| `skip` | Behavior when the timeout elapses without a matching event |

The received event payload becomes the chain value after the `wait` step and is accessible as `event.*` in subsequent CEL expressions.

### Emit + Wait Pattern

```yaml
# examples/events.flow.yaml
participants:
  publish:
    type: emit
    event: order.created
    payload:
      orderId: "ORD-001"
      total: 99.95

flow:
  - publish

  - wait:
      event: order.created
      match: event.orderId == "ORD-001"
      timeout: 5s
      onTimeout: fail

  - type: exec
    run: echo "Received order ${orderId} with total ${total}"
    input:
      orderId: event.orderId
      total: event.total

output: event.orderId
```

```bash
duckflux run examples/events.flow.yaml
```

### Acknowledged Emit Pattern

```yaml
# examples/events-ack.flow.yaml
participants:
  publisher:
    type: emit
    event: payment.processed
    ack: true
    timeout: 5s
    onTimeout: fail
    payload:
      transactionId: "TXN-42"
      status: "approved"

flow:
  - wait:
      event: payment.processed
      timeout: 5s
      onTimeout: skip

  - publisher

output: publisher.output.ack
```

```bash
duckflux run examples/events-ack.flow.yaml
```

### NATS Backend: Pre-creating Streams

NATS JetStream stream names cannot contain dots, but duckflux event names use dot-notation (e.g. `payment.processed`). The runner handles this transparently by looking up the stream by subject (`StreamNameBySubject`) instead of by name.

You must create streams before running. Replace dots with underscores in the stream name; use the original dot-notation event name as the subject:

```bash
nats stream add payment_processed \
  --subjects "payment.processed" \
  --storage file \
  --replicas 1 \
  --server nats://localhost:4222
```

Then run:

```bash
duckflux run workflow.flow.yaml \
  --event-backend=nats \
  --nats-url=nats://localhost:4222
```

### Sub-Workflow Hub Sharing

The hub is shared recursively with all sub-workflows. An event emitted by the parent is visible to sub-workflows and vice versa — they all use the same underlying pub/sub bus.

```yaml
# parent.flow.yaml
participants:
  child:
    type: workflow
    path: child.flow.yaml

  trigger:
    type: emit
    event: job.started
    payload: {jobId: "42"}

flow:
  - trigger    # publishes job.started to the shared hub
  - child      # child.flow.yaml can wait.event on job.started
```

To isolate a sub-workflow's events from the parent, run it as a separate `duckflux run` process instead of a `workflow` participant.

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
