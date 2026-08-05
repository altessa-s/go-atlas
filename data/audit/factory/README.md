# factory

```go
import "github.com/altessa-s/go-atlas/data/audit/factory"
```

Package `factory` provides a fluent builder for creating an audit auditor from configuration.
`AuditorBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

The builder creates the whole pipeline from configuration — `storage` picks the backend, `dispatch` tunes the engine in front of it:

```go
auditor, err := factory.New(cfg.Audit).
    UseLogger(logger).
    Build()
```

Supply a dispatcher only to override that:

```go
auditor, err := factory.New(cfg.Audit).
    UseLogger(logger).
    UseDispatcher(eng). // takes `storage` and `dispatch` out of play
    Build()
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates an `AuditorBuilder` for the given audit config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseDispatcher` | Sets the `audit.Dispatcher` (must already be started); overrides `storage` and `dispatch` |
| `UseMongoDatabase` | Sets the database for `storage.type: mongo` |
| `UseShutdownHooks` | Sets the `runtime.HookGroup` the builder registers what it owns into |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles, starts, and returns the audit auditor |

## Storage

| `storage.type` | Requires | Notes |
|----------------|----------|-------|
| `memory`       | --       | In-process ring buffer; events are lost on restart |
| `mongo`        | `UseMongoDatabase` | `storage.mongo` tunes collection name, index timeout and TTL; omitting the section means all defaults |

A `mongo` type without a database fails with `ErrMongoDatabaseRequired` rather than silently falling back to memory.

## Ownership and shutdown

When the builder creates the engine it also owns it, so it registers the engine's shutdown — an engine nobody stops keeps its workers and, with WAL
enabled, its segment files open past the point the caller believes the subsystem is gone. `shutdownTimeout` bounds the drain.

Those hooks go into the process-wide registry by default, which runs once for the whole program. Pass a
[`runtime.HookGroup`](../../../core/runtime) via `UseShutdownHooks` when the subsystem must be stoppable on its own; the auditor is placed in the
same scope, so stopping it stops the whole pipeline rather than half of it.

An injected dispatcher belongs to the caller and is never shut down by the builder.
