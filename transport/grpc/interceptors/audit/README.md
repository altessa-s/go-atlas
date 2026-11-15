# audit

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/audit"
```

Package `audit` provides a gRPC server interceptor for automatic request auditing. Uses the driven interceptor pattern and accepts an
`EventEmitter` for non-blocking, fire-and-forget event emission. `audit.Auditor` implements the interface. Extracts trace IDs, request IDs,
and client IP from context.

## Key types

| Type | Description |
|---|---|
| `EventEmitter` | Emits an `*audit.Event`; the boolean result is best effort |

## Options

| Option               | Default | Description                                            |
|----------------------|---------|--------------------------------------------------------|
| `WithActorExtractor` | nil     | Function to extract `audit.Actor` from gRPC context    |
| `WithIgnoreMethods`  | --      | Methods to skip auditing                               |
| `WithIgnorePatterns` | --      | Regex patterns for methods to skip                     |
| `WithLogger`         | discard | Structured logger (`*slog.Logger`)                     |
| `WithMaxMethodLength` | 1024   | Cap, in bytes, of the stored service and method names  |
| `WithMaxUserAgentLength` | 512 | Cap, in bytes, of the stored actor User-Agent          |
| `WithMaxMessageLength` | 1024  | Cap, in bytes, of the stored gRPC status message       |

The method names, the actor's User-Agent and the status message are client-controlled or echo client input. Each is truncated to its cap and marked
with `TruncationSuffix` (`...`), never rejected; the caller still receives the full status.
