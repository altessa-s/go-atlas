# factory

```go
import "github.com/altessa-s/go-atlas/observability/metrics/factory"
```

Package `factory` provides configuration-based creation of `metrics.Collector` instances. Reads from
`config.Metrics` to select the adapter, namespace, and other settings.

## Usage

```go
f := factory.New(factory.WithLogger(logger))
collector, err := f.CreateFromConfig(&cfg.Observability.Metrics)
if err != nil {
    return err
}
defer collector.Shutdown(ctx)
```
