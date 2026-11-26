# prometheus

```go
import "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
```

Package `prometheus` provides a Prometheus adapter for the Atlas metrics system. Translates abstract metric operations to `client_golang` types and
exposes an HTTP handler for the `/metrics` endpoint. Supports custom registries via options.

Mount `adapter.Handler()` to scrape the gatherer configured on that adapter. Use `WithRegistry` to pair a private registry's registerer and gatherer.
Observations with mismatched labels on the unbound recording path are dropped without panicking and counted by `Adapter.DroppedObservations()`.
