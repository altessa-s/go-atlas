# metrics

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"
```

Package `metrics` provides metrics collection for gRPC servers via the `observability/metrics.Collector` abstraction. Default metrics:
requests_total, request_duration_seconds, requests_in_flight, requests_in_flight_by_method. Optional: request_size_bytes, response_size_bytes,
stream messages.

## Default metrics

| Metric                                | Type      | Labels         | Description                    |
|---------------------------------------|-----------|----------------|--------------------------------|
| `server_requests_total`               | Counter   | method, status | Total gRPC requests            |
| `server_request_duration_seconds`     | Histogram | method, status | Request duration in seconds    |
| `server_requests_in_flight`           | Gauge     | --             | Current concurrent requests    |
| `server_requests_in_flight_by_method` | Gauge     | method         | Concurrent requests per method |

Metric names are prefixed with the collector's service name and the configured subsystem (default `grpc`), so the full metric becomes
`<service>_grpc_server_requests_total`.

## Optional metrics (enabled via options)

| Metric                                  | Type      | Labels         | Enabled by                       |
|-----------------------------------------|-----------|----------------|----------------------------------|
| `server_request_size_bytes`             | Histogram | method, status | `WithEnableSizeMetrics`          |
| `server_response_size_bytes`            | Histogram | method, status | `WithEnableSizeMetrics`          |
| `server_stream_messages_sent_total`     | Counter   | method, status | `WithEnableStreamMetrics`        |
| `server_stream_messages_received_total` | Counter   | method, status | `WithEnableStreamMetrics`        |
| `server_stream_message_size_bytes`      | Histogram | method, dir    | `WithEnableStreamMetrics` + size |

## Options

| Option                       | Default                 | Description                                                                                            |
|------------------------------|-------------------------|--------------------------------------------------------------------------------------------------------|
| `WithCollector`              | `metrics.Noop()`        | Metrics collector to register with; required for actual emission                                       |
| `WithMetricsSubsystem`       | `grpc`                  | Subsystem prefix; override per upstream so multiple servers can share one registry                     |
| `WithEnableSizeMetrics`      | false                   | Enable request/response size histograms                                                                |
| `WithEnableStreamMetrics`    | false                   | Enable per-stream message counters                                                                     |
| `WithDurationBuckets`        | default latency buckets | Custom duration histogram buckets (seconds)                                                            |
| `WithSizeBuckets`            | default size buckets    | Custom size histogram buckets (bytes)                                                                  |
| `WithStreamSamplingRate`     | 1.0                     | Sampling rate for stream metrics (0.0–1.0)                                                             |
| `WithStreamSamplingStrategy` | PerMessage              | Sampling strategy: PerMessage or PerStream                                                             |
| `WithIgnoreMethods`          | --                      | Methods to skip metrics collection                                                                     |
| `WithIgnorePatterns`         | reflection, health      | Regex patterns for methods to skip                                                                     |
| `WithLogger`                 | discard                 | Structured logger                                                                                      |
