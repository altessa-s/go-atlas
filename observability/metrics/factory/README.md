# factory

```go
import "github.com/altessa-s/go-atlas/observability/metrics/factory"
```

Package `factory` provides configuration-based creation of `metrics.Collector` instances. Reads from `config.Metrics` to select the
adapter, namespace, subsystem defaults, and other settings required for production metrics collection.
