# duckflux runner

> [!WARNING]
> **This runner is no longer maintained.** The Go implementation has been abandoned in favor of the JavaScript runtime.
> Please use the [JavaScript Runtime](https://docs.duckflux.openvibes.tech/runtimes/javascript) instead — it is the actively maintained runtime for duckflux.

### Why

Why JavaScript/TypeScript (Bun) instead of Go?
The original Go runner was built for a specific model: a batteries-included CLI with a fixed set of built-in backends and participant types. That worked well for v1, but duckflux's direction is toward composable, extensible workflows — custom participant types, pluggable event hub backends, and third-party integrations. Go's plugin ecosystem is fundamentally limited for this: the native plugin package is Linux-only, gRPC sidecars add operational overhead disproportionate to simple participants, and compile-time composition forces users to write their own main.go — all unacceptable trade-offs for a project that prioritizes developer experience.
The TypeScript runtime (@duckflux/runtime) solves this natively through npm's package model. Plugins are just imports: a Kafka event hub, a Slack notifier, or a custom participant type are regular npm packages that plug into the runner via a standard interface. This makes duckflux extensible by default — anyone can publish a plugin without forking the runtime or dealing with build toolchain constraints. The trade-off is losing Go's reference CEL implementation (cel-go) in favor of the community cel-js, and giving up zero-dependency static binaries. For a project whose core value proposition is composability and minimal friction for workflow authors, that trade-off is worth it.
