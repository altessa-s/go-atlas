# factory

```go
import promfactory "github.com/altessa-s/go-atlas/factory/observability/metrics/adapters/prometheus"
```

Maps the Prometheus block of the metrics configuration to a [Prometheus adapter](../../../../../observability/metrics/adapters/prometheus/) for
[`factory/observability/metrics`](../..), which knows no concrete adapter.

## Usage

```go
collector, err := metricsfactory.New(&cfg.Metrics).
    UseAdapter(observabilityconfig.MetricsTypePrometheus, promfactory.Builder()).
    Build()
```

| Function    | Description                                                                                                  |
|-------------|--------------------------------------------------------------------------------------------------------------|
| `Builder()` | Adapter builder: a private registry when `adapters.prometheus.customRegistry` is set, the default one otherwise |
