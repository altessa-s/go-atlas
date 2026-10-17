# adapters

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters"
```

Package `adapters` defines the `Adapter` interface for tracing backends. Implement this interface to translate `SpanData` into a specific backend
format. Use `MultiAdapter` to export spans to multiple destinations sequentially, stopping at the first error.

## Key types

| Type            | Description                                                  |
|-----------------|--------------------------------------------------------------|
| `Adapter`       | Interface: `Name`, `ExportSpans`, `Shutdown`, `ForceFlush`   |
| `MultiAdapter`  | Exports spans sequentially; stops at the first error         |
| `SpanData`      | Exported span information passed to adapters                 |

## Subpackages

| Package              | Description                                    |
|----------------------|------------------------------------------------|
| [console](./console) | Human-readable or JSON output to stdout/stderr |
| [otlp](./otlp)       | OTLP export over gRPC or HTTP                  |
