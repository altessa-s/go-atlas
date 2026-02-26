# otlp

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"
```

Package `otlp` provides an OpenTelemetry Protocol adapter for the Atlas tracing system. Exports spans over gRPC using the OTLP specification.

## Usage

```go
adapter, err := otlp.New(ctx,
    otlp.WithEndpoint("localhost:4317"),
    otlp.WithInsecure(),
)
if err != nil {
    return err
}

provider := tracing.New(tracing.WithAdapter(adapter))
defer provider.Shutdown(ctx)
```
