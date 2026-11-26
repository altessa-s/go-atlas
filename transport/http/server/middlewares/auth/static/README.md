# static

```go
import (
    "github.com/altessa-s/go-atlas/auth/static"
    staticadapter "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/static"
)
```

Thin HTTP adapter over [`auth/static`](../../../../../../auth/static). Provides `AuthFunc` (with an optional
`WithAudit` decision recorder), which wraps store errors with [`auth.ErrUnauthorized`](..) while preserving the original cause for logs and custom
`auth.ErrorHandler` implementations.

## Error wrapping

| `auth/static` error    | Wrapped as                                               |
|------------------------|----------------------------------------------------------|
| `ErrTokenInvalid`      | `auth.ErrUnauthorized` (cause preserved via `errors.Is`) |
| `ErrTokenEmpty`        | `auth.ErrUnauthorized` (cause preserved)                 |
| `ErrRateLimitKeyEmpty` | `auth.ErrUnauthorized` (cause preserved)                 |
| `ErrRateLimited`       | `auth.ErrUnauthorized` (cause preserved)                 |
| anything else          | wrapped with `auth:` prefix, original cause preserved    |

Use `errors.Is(err, auth.ErrUnauthorized)` or `errors.Is(err, static.ErrTokenInvalid)` (via `auth/static`) to branch on the failure mode in custom error
handlers.

Store types, options, and constructors are provided by `github.com/altessa-s/go-atlas/auth/static`.

## Usage

### Bearer token

```go
store := static.NewInMemoryStore(
    static.WithInitialTokens(map[string]any{
        "sk_live_xxx": UserInfo{ID: "u1", Roles: []string{"admin"}},
    }),
)

middleware := auth.Middleware(auth.WithAuthFunc(staticadapter.AuthFunc(store)))
handler := middleware(yourHandler)
```

### API key

```go
extractor := auth.ExtractTokenFromHeader("X-API-Key", func(v string) (string, error) {
    return v, nil
})

middleware := auth.Middleware(
    auth.WithTokenExtractor(extractor),
    auth.WithAuthFunc(staticadapter.AuthFunc(store)),
)
```

### Rate limiting

The decorator lives in `auth/static`; wire an external `static.RateLimiter` (for example one backed by `coord/limiters/tokenbucket`) and pass the wrapped store
to `AuthFunc`:

```go
store := static.NewRateLimitedStore(inner, myLimiter, func(ctx context.Context) string {
    return extractClientIP(ctx)
})

middleware := auth.Middleware(auth.WithAuthFunc(staticadapter.AuthFunc(store)))
```

## Security notes

See the [`auth/static` README](../../../../../../auth/static/README.md) — storage, HMAC digest, and timing properties are inherited from that package.
