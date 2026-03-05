# factory

```go
import "github.com/altessa-s/go-atlas/observability/tracing/factory"
```

Package `factory` provides a fluent builder for creating tracers from configuration.
`TracerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.
Returns `tracing.Noop` when config is `nil` or tracing is disabled.

## Quick Start

```go
tracer, err := factory.New(cfg.Tracing).
    UseLogger(logger).
    Build(ctx)
```

## Supported Adapter Types

| Type | Backend | Notes |
|------|---------|-------|
| `otlp` | OpenTelemetry (gRPC/HTTP) | Default endpoint `localhost:4317` |
| `console` | Stdout | Supports pretty-print and timestamps |
| `noop` | No-op | No spans emitted |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `TracerBuilder` for the given tracing config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the tracer; returns `tracing.Noop` if disabled |
