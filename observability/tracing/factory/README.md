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

## Proxy wiring

For the `otlp` adapter with `Protocol: grpc`, `Build(ctx)` materializes
`cfg.OTLP.Proxy` (a [`config.GrpcProxy`](../../../config/grpc_proxy.go))
into `grpcclient.Option` values via `cfg.OTLP.Proxy.ClientOptions()` and
forwards them through `otlp.WithGRPCClientOptions(...)`. A nil/empty
`Proxy` block keeps grpc-go's `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` env
passthrough.

`Protocol: http` does not support YAML-driven proxy: the config validator
rejects `Protocol: http + Proxy: {...}` so misconfiguration surfaces at
load time. Use env vars instead for the HTTP exporter. See the
[Proxy guide](../../../docs/proxy.md) for full mode semantics.
