# static

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"
```

Thin gRPC adapter over [`auth/static`](../../../../../auth/static). Re-exports the store types/options and provides `AuthFunc`, which
translates `auth/static` sentinel errors into gRPC status codes for the parent [auth interceptor](..).

## Error mapping

| `auth/static` error  | gRPC status code           |
|----------------------|----------------------------|
| `ErrInvalidToken`    | `codes.Unauthenticated`    |
| `ErrEmptyToken`      | `codes.Unauthenticated`    |
| `ErrRateLimited`     | `codes.ResourceExhausted`  |
| anything else        | `codes.Internal`           |

## Re-exports

| Symbol                  | Source                                                 |
|-------------------------|--------------------------------------------------------|
| `TokenStore`            | `auth/static.TokenStore`                               |
| `InMemoryStore`         | `auth/static.InMemoryStore`                            |
| `Option`                | `auth/static.Option`                                   |
| `Metrics`               | `auth/static.Metrics`                                  |
| `RateLimiter`           | `auth/static.RateLimiter`                              |
| `RateLimitedStore`      | `auth/static.RateLimitedStore`                         |
| `KeyFunc`               | `auth/static.KeyFunc`                                  |
| `NewInMemoryStore`      | `auth/static.NewInMemoryStore`                         |
| `NewMetrics`            | `auth/static.NewMetrics`                               |
| `NewRateLimitedStore`   | `auth/static.NewRateLimitedStore`                      |
| `WithInitialTokens`     | `auth/static.WithInitialTokens`                        |
| `WithMetrics`           | `auth/static.WithMetrics`                              |
| `WithHMACKey`           | `auth/static.WithHMACKey`                              |

## Usage

### Bearer token

```go
store := static.NewInMemoryStore(
    static.WithInitialTokens(map[string]any{
        "sk_live_xxx": UserInfo{ID: "u1", Roles: []string{"admin"}},
        "sk_test_xxx": UserInfo{ID: "u2", Roles: []string{"viewer"}},
    }),
)

interceptor := auth.ServerInterceptor(
    auth.WithAuthFn(static.AuthFunc(store)),
)
```

### API key via custom header

```go
extractor := auth.ExtractTokenFromHeader("x-api-key", func(v string) (string, error) {
    return v, nil
})

interceptor := auth.ServerInterceptor(
    auth.WithTokenExtractor(extractor),
    auth.WithAuthFn(static.AuthFunc(store)),
)
```

### Rate limiting

The decorator lives in `auth/static`; wire in an external limiter implementation (for example one backed by `data/limiters/tokenbucket`)
through the consumer-side `RateLimiter` interface:

```go
store := static.NewRateLimitedStore(inner, myLimiter, func(ctx context.Context) string {
    return extractClientIP(ctx)
})

interceptor := auth.ServerInterceptor(
    auth.WithAuthFn(static.AuthFunc(store)),
)
```

## Security notes

See the [`auth/static` README](../../../../../auth/static/README.md) — the storage model, HMAC digest, and timing properties are inherited
from that package.
