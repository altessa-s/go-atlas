# logger

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
```

Package `logger` provides gRPC interceptors for request and response logging. Captures method calls, timing, gRPC status codes,
errors, request/response payloads, request IDs, trace correlation, and client IP with structured logging support via a pluggable `Logger` interface.

## Key types

| Type / Interface | Description                                                                     |
|------------------|---------------------------------------------------------------------------------|
| `Logger`         | Interface: `Log(ctx, msg, grpcCode, fields)` for structured call completion events |
| `LoggerFunc`     | Function adapter for the `Logger` interface                                     |
| `Slog`           | Ready-made implementation that maps gRPC codes to `slog.Level`                  |

## Options

| Option                      | Default            | Description                                       |
|-----------------------------|--------------------|---------------------------------------------------|
| `WithTimeFormat`            | RFC3339            | Timestamp formatting style                        |
| `WithLogResponse`           | false              | Include response payloads in log output            |
| `WithLogRequest`            | false              | Include request payloads in log output             |
| `WithIgnoreMethods`         | --                 | Methods to skip logging                            |
| `WithIgnorePatterns`        | reflection, health | Regex patterns for methods to skip                 |
| `WithLogGrpcResponseCodes`  | default codes      | gRPC response codes to always log                  |
| `WithIgnoreGrpcResponseCodes` | --               | gRPC response codes to never log (takes precedence) |
| `WithContextLogger`         | nil                | Store enriched logger in context for handlers       |
