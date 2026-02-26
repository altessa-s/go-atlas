# propagation

```go
import "github.com/altessa-s/go-atlas/observability/tracing/propagation"
```

Package `propagation` implements W3C Trace Context and Baggage propagation for distributed tracing.
Injects and extracts trace context from carriers (typically HTTP headers).

## Key types

| Type / Function         | Description                                             |
|-------------------------|---------------------------------------------------------|
| `TextMapPropagator`     | Interface: `Inject`, `Extract`, `Fields`                |
| `TraceContext`          | W3C `traceparent` and `tracestate` propagator           |
| `HeaderCarrier`         | `http.Header` adapter for `TextMapCarrier`              |

## Usage

```go
propagator := propagation.TraceContext{}

// Inject into outgoing request
propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

// Extract from incoming request
ctx = propagator.Extract(ctx, propagation.HeaderCarrier(req.Header))
```
