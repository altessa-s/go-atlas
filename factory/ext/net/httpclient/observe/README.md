# observe

```go
import "github.com/altessa-s/go-atlas/factory/ext/net/httpclient/observe"
```

Package `observe` implements a [`client.Observer`](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client/README.md#observer) that records the HTTP client's events as metrics and derives health checks
from them, so the client package itself stays free of the observability stack. Create one `Observer` per client and install it with
`client.WithObserver`.

## Usage

```go
coord := health.New()
defer coord.Close()

c := client.New(
    client.WithObserver(observe.New(meter, coord,
        observe.WithMetricsSubsystem("payments"),
        observe.WithPerHostHealthChecks(),
    )),
    client.WithCircuitBreakerSettings("api.example.com", &client.CircuitBreakerSettings{Name: "api"}),
)
```

`New(meter, reporter, opts...)`: a nil meter records no metrics, a nil reporter disables the health integration.

## Options

| Option                      | Default       | Description                                                                                            |
|-----------------------------|---------------|--------------------------------------------------------------------------------------------------------|
| `WithMetricsSubsystem`      | `http_client` | Prometheus subsystem of the metrics; override per upstream so several clients can share one registry   |
| `WithHealthServiceName`     | `http_client` | Service name of the aggregate health checker                                                           |
| `WithPerHostHealthChecks`   | off           | Also register `<service>.<host>` checkers for hosts configured via `client.WithCircuitBreakerSettings` |
| `WithHealthRetryWindow`     | 60s           | Rolling window for the retry-rate contribution to health status                                        |
| `WithHealthRetryBuckets`    | 6             | Number of buckets in the retry-rate ring                                                               |
| `WithHealthRetryThreshold`  | 0.20          | Retry-rate fraction above which the aggregate reports `Degraded`                                       |
| `WithHealthRetryMinSamples` | 10            | Minimum requests in the window before retry rate is allowed to lower status                            |

## Metrics

| Metric                                    | Labels                   | Description                                                                |
|-------------------------------------------|--------------------------|----------------------------------------------------------------------------|
| `<subsystem>_requests_total`              | `method`, `status_class` | Completed requests                                                         |
| `<subsystem>_request_errors_total`        | `method`                 | Requests that failed after all retries                                     |
| `<subsystem>_retries_total`               | --                       | Retry attempts                                                             |
| `<subsystem>_request_duration_seconds`    | `method`                 | Request duration, retries and backoff included                             |
| `<subsystem>_circuit_breaker_trips_total` | --                       | Attempts rejected by a circuit breaker                                     |
| `<subsystem>_circuit_breaker_state`       | `host`                   | Breaker state (0 closed, 1 half-open, 2 open); the client-wide one by name |

## Health

The aggregate checker is registered when the observer is attached to a client (in `client.New`) and reports:

| Status       | When                                                                              |
|--------------|-----------------------------------------------------------------------------------|
| `Serving`    | every breaker is closed AND retry rate ≤ threshold (or sample below `MinSamples`) |
| `Degraded`   | any half-open, ≥ 1 open while ≥ 1 closed exists, OR retry rate above threshold    |
| `NotServing` | every breaker is open (and at least one breaker exists)                           |

The breakers counted are the per-host ones, plus the client-wide overflow breaker once it has served a request. Coordinator subscribers receive an
immediate push on every breaker state transition (the recompute runs in a goroutine because the breaker callback holds the breaker's internal mutex);
pull-based callers (`/healthz`, `/readyz`, gRPC `Check`) read the cached value through `CheckStatus`. Lazy hosts (no explicit settings) only
contribute to the aggregate.

## Key types

| Type       | Description                                                        |
|------------|--------------------------------------------------------------------|
| `Observer` | `client.Observer` implementation returned by `New`; one per client |
| `Option`   | Generated functional option                                        |

## See also

- [client](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client/README.md) — the resilient HTTP client and its `Observer` events
- [factory](../README.md) — `HealthOptions` maps the client-health configuration schema to observer options
