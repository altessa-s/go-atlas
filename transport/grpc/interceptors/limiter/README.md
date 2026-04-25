# limiter

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/limiter"
```

Package `limiter` provides gRPC interceptors for rate limiting. Server-side injects standard rate-limit response headers (`x-ratelimit-limit`,
`x-ratelimit-remaining`, `x-ratelimit-reset`). Returns `codes.ResourceExhausted` on limit exceeded. Configurable fallback behavior on failure.

## Key types

| Type / Interface | Description                                                                        |
|------------------|------------------------------------------------------------------------------------|
| `Limiter`        | Uses `data/limiters.Limiter` interface: `Limit(ctx) (*LimitInfo, error)`           |

## Server options

| Option                 | Default            | Description                                         |
|------------------------|--------------------|-----------------------------------------------------|
| `WithLogger`           | discard            | Structured logger                                   |
| `WithIgnoreMethods`    | --                 | Methods to skip rate limiting                       |
| `WithIgnorePatterns`   | reflection, health | Regex patterns for methods to skip                  |
| `WithFallbackBehavior` | Deny               | Behavior on limiter failure (Allow, Deny, or Error) |

## Client options

| Option                          | Default       | Description                                    |
|---------------------------------|---------------|------------------------------------------------|
| `WithClientLogger`              | `slog.Default`| Structured logger                              |
| `WithClientFallbackBehavior`    | Deny          | Behavior on limiter failure                    |
