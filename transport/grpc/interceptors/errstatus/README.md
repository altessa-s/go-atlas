# errstatus

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
```

Package `errstatus` provides gRPC interceptors for consistent error-to-status and status-to-error conversion. Server-side: `ErrorConverter`
mappings with optional `Finalizer` and internal LRU cache. Client-side: `StatusConverter` for translating gRPC statuses back to application errors.

## Key types

| Type / Interface  | Description                                                                     |
|-------------------|---------------------------------------------------------------------------------|
| `ErrorConverter`  | Server-side: Matcher + Convert function for transforming errors to gRPC status  |
| `StatusConverter` | Client-side: Matcher + Convert function for translating statuses to errors      |
| `Finalizer`       | Post-processing function for enriching converted errors (e.g., ErrorInfo)       |
| `StatusError`     | Interface for service-level custom error conversion                             |

## Server options

| Option                     | Default               | Description                                       |
|----------------------------|-----------------------|---------------------------------------------------|
| `WithErrorConverters`      | --                    | Custom error-to-status converters (priority order) |
| `WithErrorMapping`         | --                    | Simple error-to-code mapping via `errors.Is`      |
| `WithErrorTypeMapping`     | --                    | Type-safe converter using `errors.As` with generics |
| `WithFinalizer`            | nil                   | Post-processing for converted errors              |
| `WithCacheSize`            | 1000                  | Maximum entries in conversion LRU cache           |
| `WithCacheDisabled`        | false                 | Completely disable error conversion caching        |
| `WithCacheOnlySentinel`    | false                 | Restrict caching to sentinel errors only          |
| `WithSentinelErrors`       | --                    | Additional sentinel errors eligible for caching    |
| `WithLogger`               | discard               | Structured logger                                  |

## Client options

| Option                     | Default | Description                                                 |
|----------------------------|---------|-------------------------------------------------------------|
| `WithStatusConverters`     | --      | Custom status-to-error converters (priority order)          |
| `WithStatusMapping`        | --      | Simple code-to-error mapping                                |
| `WithStatusConverterFunc`  | --      | Function-based status converter                             |

## Functions

| Function              | Description                                                        |
|-----------------------|--------------------------------------------------------------------|
| `DefaultFinalizer`    | Enriches errors with ErrorInfo reason codes and RequestInfo details |
| `GrpcStatusToReasonCode` | Maps gRPC status codes to standardized reason code strings      |
