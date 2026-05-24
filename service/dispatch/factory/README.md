# factory

```go
import "github.com/altessa-s/go-atlas/service/dispatch/factory"
```

Package `factory` provides a fluent builder for creating dispatch engines from configuration.
`EngineBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
engine, err := factory.New[*audit.Event](cfg.Dispatch).
    WithSink(auditStorage).
    WithCodec(audit.JSONCodec{}).
    WithLogger(logger).
    Build()
```

With WAL enabled for durability:

```go
engine, err := factory.New[Message](cfg.Dispatch).
    WithSink(messageSink).
    WithCodec(jsonCodec).
    WithLogger(logger).
    Build() // WAL auto-enabled if cfg.WAL.Dir is set
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New[T](cfg)` | Creates an `EngineBuilder` for the given dispatch config |

### Dependencies

| Method | Description |
|--------|-------------|
| `WithSink` | Sets the sink that receives dispatched batches (required) |
| `WithCodec` | Sets the codec for WAL serialization (required when WAL is enabled) |
| `WithLogger` | Sets the logger for the builder and engine |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the dispatch engine |

## Configuration

The builder consumes a [`config.Dispatch`](../../../config/dispatch.go) struct which controls:

- Batch size and flush intervals
- Worker concurrency
- WAL settings (directory, segment size, fsync interval)
- Retry policies

## WAL Support

When `cfg.WAL.Dir` is set, the builder automatically enables write-ahead logging
for durability. The WAL ensures events are persisted before dispatch and can
recover from crashes. A codec must be provided via `WithCodec` when WAL is enabled.