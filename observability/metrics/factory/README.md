# factory

```go
import "github.com/altessa-s/go-atlas/observability/metrics/factory"
```

Package `factory` provides a fluent builder for creating metrics collectors from configuration.
`CollectorBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.
Returns `metrics.Noop` when config is `nil` or metrics are disabled.

## Quick Start

```go
collector, err := factory.New(cfg.Metrics).
    UseLogger(logger).
    Build()
```

## Supported Adapter Types

| Type | Backend | Notes |
|------|---------|-------|
| `prometheus` | Prometheus | Supports custom registry via config |
| `noop` | No-op | No metrics emitted |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `CollectorBuilder` for the given metrics config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the metrics collector; returns `metrics.Noop` if disabled |
