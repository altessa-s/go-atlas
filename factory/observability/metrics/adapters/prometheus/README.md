# factory

```go
import promfactory "github.com/altessa-s/go-atlas/factory/observability/metrics/adapters/prometheus"
```

Maps the Prometheus block of the metrics configuration to a [Prometheus adapter](../../../../../observability/metrics/adapters/prometheus/) for
[`factory/observability/metrics`](../..), which knows no concrete adapter.

## Usage

```go
collector, handler, err := metricsfactory.New(&cfg.Metrics).
    UseAdapter(observabilityconfig.MetricsTypePrometheus, promfactory.Builder()).
    BuildWithHandler()
```

Mount `handler` at `/metrics` to scrape the same registry the collector writes to.

| Function    | Description                                                                                                  |
|-------------|--------------------------------------------------------------------------------------------------------------|
| `Builder()` | Adapter builder: a private registry when `adapters.prometheus.customRegistry` is set, the default one otherwise |
