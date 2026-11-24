# metrics

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metrics"
```

Package `metrics` provides metrics collection for gRPC servers and clients via the `observability/metrics.Meter` abstraction.
`ServerInterceptor` and `ClientInterceptor` each provide unary and streaming interceptors for use with `interceptors.NewChain`.
Use `ClientUnaryInterceptor` or `ClientStreamInterceptor` to register an individual interceptor with gRPC.

## Default metrics

| Metric                                | Type      | Labels         | Description                    |
|---------------------------------------|-----------|----------------|--------------------------------|
| `server_requests_total`               | Counter   | method, status | Total gRPC requests            |
| `server_request_duration_seconds`     | Histogram | method, status | Request duration in seconds    |
| `server_requests_in_flight`           | Gauge     | --             | Current concurrent requests    |
| `server_requests_in_flight_by_method` | Gauge     | method         | Concurrent requests per method |
| `client_requests_total`               | Counter   | method, status | Total outgoing gRPC calls      |
| `client_request_duration_seconds`     | Histogram | method, status | Outgoing call duration in seconds |
| `client_requests_in_flight`           | Gauge     | --             | Current concurrent outgoing calls |
| `client_requests_in_flight_by_method` | Gauge     | method         | Concurrent outgoing calls per method |

Metric names are prefixed with the collector's service name and the configured subsystem (default `grpc`), so the full metric becomes
`<service>_grpc_server_requests_total`.
Client calls use the full gRPC method and the final gRPC status code name, such as `OK`, `Unavailable`, or `DeadlineExceeded`.
Native gRPC retries are part of one outgoing call. A stream stays in flight until gRPC reports its final status.

## Optional metrics (enabled via options)

| Metric                                  | Type      | Labels         | Enabled by                       |
|-----------------------------------------|-----------|----------------|----------------------------------|
| `server_request_size_bytes`             | Histogram | method, status | `WithEnableSizeMetrics`          |
| `server_response_size_bytes`            | Histogram | method, status | `WithEnableSizeMetrics`          |
| `server_stream_messages_sent_total`     | Counter   | method, status | `WithEnableStreamMetrics`        |
| `server_stream_messages_received_total` | Counter   | method, status | `WithEnableStreamMetrics`        |
| `server_stream_message_size_bytes`      | Histogram | method, direction | `WithEnableStreamMetrics` + size |
| `client_request_size_bytes`             | Histogram | method, status | `WithEnableSizeMetrics`          |
| `client_response_size_bytes`            | Histogram | method, status | `WithEnableSizeMetrics`          |
| `client_stream_messages_sent_total`     | Counter   | method, status | `WithEnableStreamMetrics`        |
| `client_stream_messages_received_total` | Counter   | method, status | `WithEnableStreamMetrics`        |
| `client_stream_message_size_bytes`      | Histogram | method, direction | `WithEnableStreamMetrics` + size |

Client stream direction is `sent` or `received` from the caller's perspective.

## Options

| Option                       | Default                 | Description                                                                                            |
|------------------------------|-------------------------|--------------------------------------------------------------------------------------------------------|
| `WithMeter`                  | `metrics.Noop()`        | Metrics collector to register with; unset records nothing                                              |
| `WithMetricsSubsystem`       | `grpc`                  | Subsystem prefix for grouping metrics by server or upstream client                                      |
| `WithEnableSizeMetrics`      | false                   | Enable request/response size histograms                                                                |
| `WithEnableStreamMetrics`    | false                   | Enable per-stream message counters                                                                     |
| `WithDurationBuckets`        | default latency buckets | Custom duration histogram buckets (seconds)                                                            |
| `WithSizeBuckets`            | default size buckets    | Custom size histogram buckets (bytes)                                                                  |
| `WithStreamSamplingRate`     | 1.0                     | Sampling rate for stream metrics (0.0–1.0)                                                             |
| `WithStreamSamplingStrategy` | `PerMessageSampling`    | Sampling strategy: `PerMessageSampling` or `PerStreamSampling`                                         |
| `WithMaxMethods`             | 1024                    | Cap on distinct `method` label values; later methods are recorded as `other`                           |
| `WithIgnoreMethods`          | --                      | Methods to skip metrics collection                                                                     |
| `WithIgnorePatterns`         | reflection, health      | Regex patterns for methods to skip                                                                     |
| `WithLogger`                 | discard                 | Structured logger                                                                                      |
