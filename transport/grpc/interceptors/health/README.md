# health

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/health"
```

Package `health` provides a gRPC server interceptor for service health checking. Rejects requests with `codes.Unavailable` when the `Health`
check fails. Wraps errors with `ErrServiceUnavailable`. If the Health check returns a gRPC status error, it is forwarded as-is.

## Key types

| Type / Interface        | Description                                                            |
|-------------------------|------------------------------------------------------------------------|
| `Health`                | Interface with a single method: `Health(context.Context) error`        |
| `ErrServiceUnavailable` | Sentinel error indicating the service cannot handle requests           |

## Options

| Option               | Default            | Description                           |
|----------------------|--------------------|---------------------------------------|
| `WithLogger`         | discard            | Structured logger                     |
| `WithIgnoreMethods`  | --                 | Methods to skip health checking       |
| `WithIgnorePatterns` | reflection, health | Regex patterns for methods to skip    |
