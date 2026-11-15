# observability

Observability subsystem for the Atlas framework. Provides metrics collection, distributed tracing, structured logging, health checks, and application
statistics — all behind abstract interfaces with pluggable backends and zero-cost no-op defaults.

## Packages

| Package                | Description                                                                     |
|------------------------|---------------------------------------------------------------------------------|
| [appstats](./appstats) | CPU, memory, network I/O, and goroutine metrics with background caching         |
| [health](./health)     | Transport-agnostic health check coordination with subscriptions                 |
| [metrics](./metrics)   | Abstract metrics (Counter, Gauge, Histogram, Timer) with adapters               |
| [tracing](./tracing)   | Abstract distributed tracing with adapters and sampling                         |

## Design principles

- **Backend-agnostic** — components depend on abstract interfaces; concrete backends are injected via adapters.
- **Optional by default** — metrics and tracing provide `Noop()` implementations so consumers can default to zero-cost no-ops; logging can
  use `slog.DiscardHandler`.
- **Factory-driven** — health, metrics and tracing have a configuration-based builder under [`factory/observability`](../factory/observability/);
  loggers from [go-slogx](https://github.com/altessa-s/go-slogx) are built by [`factory/ext/slogx`](../factory/ext/slogx/).
- **Allocation-aware** — object pooling for labels, attributes, and buffers to reduce GC pressure.
- **Composable** — tracing and metrics support scoped naming.
