# oidc

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
```

Thin gRPC adapter over [`auth/oidc`](../../../../../auth/oidc/README.md). `AuthFunc` runs an `auth/oidc.ClaimsValidator` (typically
`auth/oidc.NewValidator(provider)`) against the request's bearer token, returns the verified `*auth/oidc.Claims` as `Credentials.Data`, and maps every
validation failure to `codes.Unauthenticated`. The claims model, claim extraction and `ScopesOf` live in `auth/oidc` and are shared with the
[HTTP middleware adapter](../../../../http/server/middlewares/auth/oidc).

## Key types

| Symbol      | Description                                                                              |
|-------------|------------------------------------------------------------------------------------------|
| `AuthFunc`  | Creates an `auth.AuthFunc` from an `auth/oidc.ClaimsValidator`; failures → `Unauthenticated` |
| `AuthOption`| Configures `AuthFunc`                                                                    |
| `WithAudit` | Records each token-validation decision (action `validate_token`) through `auth.DecisionRecorder` |

## Error mapping

| Failure                            | gRPC status       | Audit reason    |
|------------------------------------|-------------------|-----------------|
| No bearer token in the request     | `Unauthenticated` | `missing_token` |
| Validator error wrapping `ErrTokenRevoked` | `Unauthenticated` | `revoked` |
| Validator error wrapping `ErrTokenInvalid` | `Unauthenticated` | `invalid_token` |
| Any other validator error          | `Unauthenticated` | `error`         |

## Usage

```go
provider, err := authoidc.NewProvider(ctx, discoveryURL, authoidc.WithHTTPClient(httpclient.New()))
if err != nil {
    return err
}

interceptor := auth.ServerInterceptor(
    auth.WithAuthFn(grpcoidc.AuthFunc(authoidc.NewValidator(provider))),
)
```

Read the claims in a handler with `auth.CredentialsFromContext(ctx)` and a `creds.Data.(*authoidc.Claims)` assertion; build a scope enforcer with
`scope.ScopeAuthorizer(authoidc.ScopesOf, scope.Exact())`. The interceptor also stores the caller as an
[`auth/principal`](../../../../../auth/principal) `Principal` mapped by `Claims.Principal` (default `tenant` and `roles` claims), so
`principal.FromContext(ctx)` works on the OIDC path too.
