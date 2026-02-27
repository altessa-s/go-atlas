# metrics

```go
import "github.com/altessa-s/go-atlas/observability/metrics"
```

Package `metrics` provides an abstract metrics collection system. Components depend on abstract interfaces (`Counter`, `Gauge`,
`Histogram`, `Timer`); export happens through pluggable adapters such as Prometheus, StatsD, or OpenTelemetry.

## Key types

| Type / Function | Description                                                 |
|-----------------|-------------------------------------------------------------|
| `Collector`     | Create and manage metrics; supports `WithSubsystem` scoping |
| `Counter`       | Monotonically increasing value                              |
| `Gauge`         | Value that can increase and decrease                        |
| `Histogram`     | Distribution of observed values                             |
| `Timer`         | Duration measurement with `Start()` / `ObserveDuration()`   |
| `Labels`        | `map[string]string` for metric dimensions                   |
| `Noop()`        | Zero-cost no-op implementation                              |

## Naming convention

Full metric name: `{namespace}_{subsystem}_{name}` (e.g., `myapp_broker_requests_total`).
Subsystem is optional and defaults to empty.

## Subpackages

| Package                                      | Description                                      |
|----------------------------------------------|--------------------------------------------------|
| [adapters](./adapters)                       | Adapter interface and `MultiAdapter` broadcaster |
| [adapters/prometheus](./adapters/prometheus)  | Prometheus backend with HTTP `/metrics` handler  |
| [factory](./factory)                         | Configuration-based `Collector` creation         |
