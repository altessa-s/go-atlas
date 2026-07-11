# limiter

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/limiter"
```

Package `limiter` provides gRPC interceptors for rate limiting. On limit exceeded it returns `codes.ResourceExhausted`, with configurable fallback
behavior on limiter failure. Standard rate-limit response headers (`x-ratelimit-limit`, `x-ratelimit-remaining`, `x-ratelimit-reset`) reveal the
configured capacity, so they are **withheld by default** and emitted only when `WithExposeHeaders` is set — rate limiting itself is unaffected. The
interceptor also declares `auth` as an ordering dependency so, when an auth interceptor is present, authentication runs first (per-principal limiting;
unauthenticated requests rejected before consuming limiter budget).

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
| `WithExposeHeaders`    | false (off)        | Emit `x-ratelimit-*` response headers               |

## Client options

| Option                          | Default       | Description                                    |
|---------------------------------|---------------|------------------------------------------------|
| `WithClientLogger`              | `slog.Default`| Structured logger                              |
| `WithClientFallbackBehavior`    | Deny          | Behavior on limiter failure                    |
