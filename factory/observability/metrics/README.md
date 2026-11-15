# factory

```go
import "github.com/altessa-s/go-atlas/factory/observability/metrics"
```

Package `factory` provides a fluent builder for creating metrics collectors from configuration. `CollectorBuilder` uses deferred error accumulation —
errors from any step are collected and returned at `Build()` time. Returns `metrics.Noop` when config is `nil` or metrics are disabled.

## Quick Start

```go
collector, err := factory.New(cfg.Metrics).
    UseAdapter(observabilityconfig.MetricsTypePrometheus, promfactory.Builder()).
    UseLogger(logger).
    Build()
```

The builder knows no concrete adapter: register one per metrics type with `UseAdapter`. A configured type without a registered builder fails at
`Build()` with an error naming the module to add.

## Supported Adapter Types

| Type | Backend | Notes |
|------|---------|-------|
| `prometheus` | Prometheus | Register [`factory/observability/metrics/adapters/prometheus`](adapters/prometheus)`.Builder()`; custom registry via config |
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
| `UseDefaultLogger` | Uses `slog.Default()` as the logger |
| `UseAdapter(typ, builder)` | Registers the adapter builder of one metrics type |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the metrics collector; returns `metrics.Noop` if disabled |
