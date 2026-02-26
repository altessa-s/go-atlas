# factory

```go
import "github.com/altessa-s/go-atlas/observability/health/factory"
```

Package `factory` provides configuration-based creation of `health.Coordinator` instances. Reads from
`config.Health` to set cache TTL, check timeout, shard count, and other coordinator options.

## Usage

```go
f := factory.New(factory.WithLogger(logger))
coordinator, err := f.CreateCoordinatorFromConfig(&cfg.Health)
if err != nil {
    return err
}
defer coordinator.Close()
```
