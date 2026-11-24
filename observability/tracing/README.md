# tracing

```go
import "github.com/altessa-s/go-atlas/observability/tracing"
```

Package `tracing` provides an abstract distributed tracing system. Components depend on abstract interfaces (`Tracer`, `Recorder`, `Span`); export
happens through pluggable adapters (OTLP, console, etc.) with configurable sampling strategies.

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

## Options

`New` takes the generated `Option` values (`WithServiceName`, `WithAdapter`, `WithSampler`, ...). Recorders, spans and events have their own generated
option types; a custom `Tracer`, `Recorder` or `Span` resolves them with `ApplyRecorderOptions`, `ApplySpanStartOptions`, `ApplySpanEndOptions` and
`ApplyEventOptions`, which return read-only `RecorderConfig`, `SpanStartConfig`, `SpanEndConfig` and `EventConfig` snapshots.

| Option type       | Options                                                              | Used by           |
|-------------------|----------------------------------------------------------------------|-------------------|
| `RecorderOption`  | `WithInstrumentationVersion`, `WithSchemaURL`                        | `Tracer.Recorder` |
| `SpanStartOption` | `WithSpanKind`, `WithAttributes`, `WithStartTimestamp`, `WithLinks`  | `Recorder.Start`  |
| `SpanEndOption`   | `WithEndTimestamp`                                                   | `Span.End`        |
| `EventOption`     | `WithEventTimestamp`, `WithEventAttributes`, `WithStackTrace`        | `Span.AddEvent`   |

`WithAttributes`, `WithLinks` and `WithEventAttributes` accumulate across repeated options; the others are last-wins.

## Batch export

`Span.End` timestamps a sampled span and tries to enqueue it. One worker sends full batches immediately and partial batches on a timer.
A full queue drops the newly ended span silently. Unsampled spans bypass the queue. The tracer submits each accepted span to the adapter at most
once. Export failures discard the batch, and ordering across batches is not guaranteed. Transport-level retries follow the adapter's policy.

| Tracer option | Optional YAML key under `tracing.batch` | Default |
|---------------|----------------------------------------|---------|
| `WithMaxQueueSize` | `maxQueueSize` | 2048 waiting spans |
| `WithMaxExportBatchSize` | `maxExportBatchSize` | 512 spans, capped at the queue size |
| `WithScheduledDelay` | `scheduledDelay` | 1s |
| `WithExportTimeout` | `exportTimeout` | 10s per batch |

All four settings must be positive. The config validator rejects an explicit batch size larger than the queue. Direct Go options ignore
nonpositive values and cap the effective batch size at the queue size. The worker can hold one batch in flight in addition to the queue.

Use `tracer.ForceFlush(ctx)` before reading exported data. It waits for spans already enqueued, including an in-flight export, then flushes the
adapter. A flush reports the most recent batch failure since the previous flush and any adapter flush error. The caller can bound the wait with
its context. At service exit, call `tracer.Shutdown(ctx)` with a deadline. The first shutdown closes admission, drains the queue within its
budget, and shuts down the adapter. Spans ended after admission closes are silently discarded. Cancellation interrupts in-flight work and
discards the remaining queue. Adapter calls must return on context cancellation for their worker to exit. A console writer that can block
indefinitely needs its own I/O deadline.

Span, event and link attributes of the built-in slice types are copied for export. Custom reference-valued attributes must remain immutable through export.

Pass `WithMeter(meter)`, or `UseMeter(meter)` on the factory builder, to record successful spans, queue-full drops, failed export calls and
queue length. The meter samples queue pressure once per second, including during a stalled export, and again at flush.
See the [metric names](../../docs/metrics.md#tracing).

## Subpackages

| Package                                    | Description                                          |
|--------------------------------------------|------------------------------------------------------|
| [adapters](./adapters)                     | Adapter interface and `MultiAdapter` broadcaster     |
| [adapters/console](./adapters/console)     | Human-readable or JSON console output                |
| [adapters/otlp](./adapters/otlp)          | OpenTelemetry Protocol export over gRPC or HTTP      |
| [factory](../../factory/observability/tracing)                       | Configuration-based `Tracer` creation                |
| [propagation](./propagation)              | W3C Trace Context propagation                        |
| [sampler](./sampler)                       | Sampling strategies: ratio, parent-based, always     |
