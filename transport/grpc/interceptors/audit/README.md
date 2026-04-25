# audit

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/audit"
```

Package `audit` provides a gRPC server interceptor for automatic request auditing. Uses the driven interceptor pattern and delegates to
`data/audit.Auditor` for non-blocking, fire-and-forget event emission. Extracts trace IDs, request IDs, and client IP from context.

## Options

| Option               | Default | Description                                            |
|----------------------|---------|--------------------------------------------------------|
| `WithActorExtractor` | nil     | Function to extract `audit.Actor` from gRPC context    |
| `WithIgnoreMethods`  | --      | Methods to skip auditing                               |
| `WithIgnorePatterns` | --      | Regex patterns for methods to skip                     |
| `WithLogger`         | discard | Structured logger (`*slog.Logger`)                     |
