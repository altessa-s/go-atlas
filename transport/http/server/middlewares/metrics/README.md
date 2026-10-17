# metrics

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/metrics"
```

Package `metrics` provides middleware that records HTTP server metrics via the `observability/metrics.Collector` abstraction. Each call to `New`
constructs a fresh middleware wired to the supplied collector; the underlying adapter deduplicates metric registrations by name, so reusing the same
collector across multiple calls with the same subsystem shares the same metric vectors.

## Metrics

Metric names are prefixed with the collector's service name and the configured subsystem (default `http`), so the full name becomes
`<service>_http_server_requests_total`.

| Metric                            | Type      | Description                                              |
|-----------------------------------|-----------|----------------------------------------------------------|
| `server_requests_total`           | counter   | Total requests by method and status code                 |
| `server_request_duration_seconds` | histogram | Request latency distribution                             |
| `server_requests_in_flight`       | gauge     | Number of concurrent in-flight requests                  |
| `server_request_size_bytes`       | histogram | Request body sizes (opt-in via `WithEnableSizeMetrics`)  |
| `server_response_size_bytes`     | histogram | Response body sizes (opt-in via `WithEnableSizeMetrics`) |

## Options

| Option                  | Default                 | Description                                                                                            |
|-------------------------|-------------------------|--------------------------------------------------------------------------------------------------------|
| `WithCollector`         | -- (required)           | Metrics collector to register with; `New` panics when it is unset                                      |
| `WithMetricsSubsystem`  | `http`                  | Subsystem prefix; override per upstream so multiple servers can share one registry                     |
| `WithEnableSizeMetrics` | false                   | Enable request/response size histograms                                                                |
| `WithDurationBuckets`   | collector default       | Custom duration histogram buckets (seconds); unset uses `observability/metrics.DefaultDurationBuckets`  |
| `WithSizeBuckets`       | collector default       | Custom size histogram buckets (bytes); unset also falls back to the duration buckets, so pass `observability/metrics.DefaultSizeBuckets` |
| `WithIgnorePaths`       | --                      | Exact URL paths to skip                                                                                |
| `WithIgnorePatterns`    | health, metrics, pprof  | Regex patterns for URL paths to skip                                                                   |
| `WithLogger`            | discard                 | Structured logger                                                                                      |
