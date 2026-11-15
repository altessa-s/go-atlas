# static

```go
import (
    "github.com/altessa-s/go-atlas/auth/static"
    staticadapter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"
)
```

Thin gRPC adapter over [`auth/static`](../../../../../auth/static). Provides `AuthFunc`, which translates
`auth/static` sentinel errors into gRPC status codes for the parent [auth interceptor](..). Missing token credentials also yield
`codes.Unauthenticated`; unknown store errors are not leaked to the client. Pass `WithAudit(recorder, subjectOf)` to record each authentication decision
through `auth.DecisionRecorder`.

## Error mapping

| `auth/static` error  | gRPC status code           |
|----------------------|----------------------------|
| `ErrTokenInvalid`    | `codes.Unauthenticated`    |
| `ErrTokenEmpty`      | `codes.Unauthenticated`    |
| `ErrRateLimited`     | `codes.ResourceExhausted`  |
| anything else        | `codes.Internal`           |

Store types, options, and constructors are provided by `github.com/altessa-s/go-atlas/auth/static`.

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
    auth.WithAuthFn(staticadapter.AuthFunc(store)),
)
```

### API key via custom header

```go
extractor := auth.ExtractTokenFromHeader("x-api-key", func(v string) (string, error) {
    return v, nil
})

interceptor := auth.ServerInterceptor(
    auth.WithTokenExtractor(extractor),
    auth.WithAuthFn(staticadapter.AuthFunc(store)),
)
```

### Rate limiting

The decorator lives in `auth/static`; wire in an external limiter implementation (for example one backed by `coord/limiters/tokenbucket`) through the
consumer-side `static.RateLimiter` interface:

```go
store := static.NewRateLimitedStore(inner, myLimiter, func(ctx context.Context) string {
    return extractClientIP(ctx)
})

interceptor := auth.ServerInterceptor(
    auth.WithAuthFn(staticadapter.AuthFunc(store)),
)
```

## Security notes

See the [`auth/static` README](../../../../../auth/static/README.md) — the storage model, HMAC digest, and timing properties are inherited from that
package.
