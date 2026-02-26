# adapters

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters"
```

Package `adapters` defines the `Adapter` interface for tracing backends. Implement this interface to translate `SpanData` into a 
specific backend format.

## Key types

| Type            | Description                                                  |
|-----------------|--------------------------------------------------------------|
| `Adapter`       | Interface: `ExportSpans`, `Shutdown`, `ForceFlush`           |
| `MultiAdapter`  | Broadcasts spans to multiple adapters simultaneously         |
| `SpanData`      | Exported span information passed to adapters                 |

## Subpackages

| Package              | Description                                    |
|----------------------|------------------------------------------------|
| [console](./console) | Human-readable or JSON output to stdout/stderr |
| [otlp](./otlp)       | OTLP export over gRPC                          |
