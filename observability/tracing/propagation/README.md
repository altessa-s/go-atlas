# propagation

```go
import "github.com/altessa-s/go-atlas/observability/tracing/propagation"
```

Package `propagation` implements W3C Trace Context propagation for distributed tracing. Injects and extracts trace context from carriers (typically HTTP
headers) using the `TextMapPropagator` interface and `HeaderCarrier` adapter.

## Key types

| Type / Function         | Description                                             |
|-------------------------|---------------------------------------------------------|
| `TextMapPropagator`     | Interface: `Inject`, `Extract`, `Fields`                |
| `TraceContext`          | W3C `traceparent` and `tracestate` propagator (`NewTraceContext`) |
| `HeaderCarrier`         | `http.Header` adapter for `TextMapCarrier`              |
| `MapCarrier`            | `map[string]string` adapter for `TextMapCarrier`        |
| `CompositePropagator`   | Combines several propagators (`NewCompositePropagator`) |
| `RemoteSpanContextFromContext` | Returns the extracted remote span context        |
