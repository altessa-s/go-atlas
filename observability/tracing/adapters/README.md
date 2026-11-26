# adapters

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters"
```

Package `adapters` defines the `Adapter` interface for tracing backends. Implement this interface to translate `SpanData` into a specific backend
format. Use `MultiAdapter` to export spans to multiple destinations sequentially: every adapter receives each call even when an earlier one
fails, and the failures are returned joined with `errors.Join`.

## Key types

| Type            | Description                                                  |
|-----------------|--------------------------------------------------------------|
| `Adapter`       | Interface: `Name`, `ExportSpans`, `Shutdown`, `ForceFlush`   |
| `ExportResult`  | Span-level `Accepted` and `Rejected` counts returned by `ExportSpans` |
| `MultiAdapter`  | Calls every adapter in order; aggregates `ExportResult` and joins errors |
| `SpanData`      | Exported span information passed to adapters                 |

## Subpackages

| Package              | Description                                    |
|----------------------|------------------------------------------------|
| [console](./console) | Human-readable or JSON output to stdout/stderr |
| [otlp](./otlp)       | OTLP export over gRPC or HTTP                  |
