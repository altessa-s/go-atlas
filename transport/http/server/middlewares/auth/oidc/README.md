# oidc

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/oidc"
```

Thin HTTP adapter over [`auth/oidc`](../../../../../../auth/oidc/README.md). `AuthFunc` runs an `auth/oidc.ClaimsValidator` (typically
`auth/oidc.NewValidator(provider)`) against the bearer token, stores the verified `*auth/oidc.Claims` as the request's authenticated data, and wraps
every failure with [`auth.ErrUnauthorized`](..) so the middleware answers 401. The claims model and extraction are shared with the
[gRPC interceptor adapter](../../../../../grpc/interceptors/auth/oidc).

## Key types

| Symbol       | Description                                                                                      |
|--------------|--------------------------------------------------------------------------------------------------|
| `AuthFunc`   | Creates an `auth.AuthFunc` from an `auth/oidc.ClaimsValidator`; panics on a nil validator        |
| `AuthOption` | Configures `AuthFunc`                                                                            |
| `WithAudit`  | Records each token-validation decision (action `validate_token`) through `auth.DecisionRecorder` |

## Error wrapping

| Failure                                    | Returned error                                   | Audit reason    |
|--------------------------------------------|--------------------------------------------------|-----------------|
| Validator error wrapping `ErrTokenRevoked` | `auth.ErrUnauthorized` (cause kept for `errors.Is`) | `revoked`       |
| Validator error wrapping `ErrTokenInvalid` | `auth.ErrUnauthorized` (cause kept)               | `invalid_token` |
| Any other validator error                  | `auth.ErrUnauthorized` (cause kept)               | `error`         |

A missing or malformed `Authorization` header is rejected by the auth middleware's token extractor before `AuthFunc` runs.

## Usage

```go
provider, err := authoidc.NewProvider(ctx, discoveryURL, authoidc.WithHTTPClient(httpclient.New()))
if err != nil {
    return err
}

middleware := auth.Middleware(
    auth.WithAuthFunc(httpoidc.AuthFunc(authoidc.NewValidator(provider))),
)
handler := middleware(yourHandler)
```

Read the claims in a handler with `auth.FromContext(r.Context()).(*authoidc.Claims)`; build a scope enforcer with
`scope.ScopeAuthorizer(authoidc.ScopesOf, scope.Exact())`. The middleware also stores the caller as an
[`auth/principal`](../../../../../../auth/principal) `Principal` mapped by `Claims.Principal` (default `tenant` and `roles` claims), so
`principal.FromContext(r.Context())` works on the OIDC path too.
