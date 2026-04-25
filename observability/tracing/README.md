# tracing

```go
import "github.com/altessa-s/go-atlas/observability/tracing"
```

Package `tracing` provides an abstract distributed tracing system. Components depend on abstract interfaces (`Tracer`, `Recorder`,
`Span`); export happens through pluggable adapters (OTLP, console, etc.) with configurable sampling strategies.

## Key types

| Type / Function        | Description                                               |
|------------------------|-----------------------------------------------------------|
| `Tracer`               | Create `Recorder` instances; supports `WithScope` scoping |
| `Recorder`             | Start spans from a context                                |
| `Span`                 | Unit of work: attributes, events, status, errors          |
| `SpanContext`           | Trace ID, span ID, and sampling flags                     |
| `Attribute`            | Key-value pair with typed helpers (`String`, `Int`, etc.) |
| `SpanFromContext`      | Extract the current span from context                     |
| `TraceIDFromContext`   | Extract the trace ID for log correlation                  |
| `Noop()`               | Zero-cost no-op implementation                            |

## Subpackages

| Package                                    | Description                                          |
|--------------------------------------------|------------------------------------------------------|
| [adapters](./adapters)                     | Adapter interface and `MultiAdapter` broadcaster     |
| [adapters/console](./adapters/console)     | Human-readable or JSON console output                |
| [adapters/otlp](./adapters/otlp)          | OpenTelemetry Protocol export over gRPC              |
| [factory](./factory)                       | Configuration-based `Tracer` creation                |
| [propagation](./propagation)              | W3C Trace Context and Baggage propagation            |
| [sampler](./sampler)                       | Sampling strategies: ratio, parent-based, always     |
