# prometheus

```go
import "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
```

Package `prometheus` provides a Prometheus adapter for the Atlas metrics system. Translates abstract metric operations to `client_golang` types and 
exposes an HTTP handler for the `/metrics` endpoint.

## Usage

```go
adapter := prometheus.New(
    prometheus.WithRegistry(reg), // optional custom registry
)

collector := metrics.New(
    metrics.WithNamespace("myapp"),
    metrics.WithAdapter(adapter),
)
```
