# factory

```go
import otlpfactory "github.com/altessa-s/go-atlas/factory/observability/tracing/adapters/otlp"
```

Maps the `otlp` block of the tracing configuration to an [OTLP adapter](../../../../../observability/tracing/adapters/otlp/) for
[`factory/observability/tracing`](../..), which builds only the console and noop adapters. It builds the exporter's client: an SSRF-guarded HTTP client
pinned to the collector (and the configured proxy) for OTLP/HTTP, a resilient gRPC client for OTLP/gRPC; the gRPC client is closed when the adapter
shuts down.

## Usage

```go
tracer, err := tracingfactory.New(&cfg.Tracing).
    UseAdapter(observabilityconfig.TracingTypeOTLP, otlpfactory.Builder()).
    Build(ctx)
```

| Function    | Description                                                                 |
|-------------|-----------------------------------------------------------------------------|
| `Builder()` | Adapter builder for the `otlp` tracing type; defaults when the block is absent |
