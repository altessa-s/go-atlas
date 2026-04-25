# prometheus

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/prometheus"
```

Package `prometheus` provides middleware that records Prometheus metrics for HTTP server requests. Collectors are registered with a
singleton pattern: the first call to `New` initializes and registers them, subsequent calls share those collectors.

## Metrics

| Metric                                    | Type      | Description                                         |
|-------------------------------------------|-----------|-----------------------------------------------------|
| `{prefix}server_requests_total`           | counter   | Total requests by method and status code             |
| `{prefix}server_request_duration_seconds` | histogram | Request latency distribution                         |
| `{prefix}server_requests_in_flight`       | gauge     | Number of concurrent in-flight requests              |
| `{prefix}server_request_size_bytes`       | histogram | Request body sizes (opt-in via `WithEnableSizeMetrics`)  |
| `{prefix}server_response_size_bytes`      | histogram | Response body sizes (opt-in via `WithEnableSizeMetrics`) |
