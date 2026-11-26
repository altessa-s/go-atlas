# adapters

```go
import "github.com/altessa-s/go-atlas/observability/metrics/adapters"
```

Package `adapters` defines the `Adapter` interface for metrics backends. Implement this interface to translate abstract metric operations into a
specific backend format (Prometheus, StatsD, OpenTelemetry, etc.). Use `MultiAdapter` to fan out to several.

## Key types

| Type                                                               | Description                                                                                         |
|--------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| `Adapter`                                                          | Interface: `Name`, `Register`, `RecordCounter`, `RecordGauge`, `RecordHistogram`                    |
| `GaugeAdder`                                                       | Optional capability: native additive gauge updates (`AddGauge`)                                     |
| `Flusher`, `Closer`                                                | Independent, optional lifecycle capabilities for buffered output and resource cleanup               |
| `HTTPHandler`                                                      | Optional capability: an `Adapter` that also serves metrics over HTTP via `Handler()`                |
| `Binder`                                                           | Optional capability: pre-resolves a (metric, label set) pair into a direct handle                   |
| `BoundCounter`, `BoundGauge`, `BoundGaugeAdder`, `BoundHistogram`  | Recording handles returned by `Binder`, one concrete label set each                                 |
| `MultiAdapter`                                                     | Forwards operations to multiple adapters, joining errors and maintaining shadow gauges when needed  |
| `Desc`                                                             | Metric descriptor passed during registration (`Clone`, `Equal`)                                     |
| `ErrDescriptorConflict`                                            | Sentinel returned when a metric name is redeclared with an incompatible descriptor                  |

## Subpackages

| Package                    | Description                                |
|----------------------------|--------------------------------------------|
| [memory](./memory)         | In-memory adapter for tests and assertions |
| [prometheus](./prometheus) | Prometheus adapter using `client_golang`   |
