# factory

```go
import "github.com/altessa-s/go-atlas/observability/tracing/factory"
```

Package `factory` provides configuration-based creation of `tracing.Tracer` instances. Reads from
`config.Tracing` to select the adapter, sampler, service name, and other settings.

## Usage

```go
f := factory.New(factory.WithLogger(logger))
provider, err := f.CreateFromConfig(ctx, &cfg.Observability.Tracing)
if err != nil {
    return err
}
defer provider.Shutdown(ctx)
```
