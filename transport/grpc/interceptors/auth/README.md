# auth

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
```

Package `auth` provides gRPC interceptors for token-based authentication and scope-based authorization. ServerInterceptor validates tokens via a
pluggable `Auth` function, ClientInterceptor injects tokens into outgoing metadata. `ScopeClientAuth` enforces the method→scope policy held in an
[`auth/scope.Registry`](../../../../auth/scope/README.md) through an `auth/scope.Enforcer`.

## Key types

| Type / Interface  | Description                                                                  |
|-------------------|------------------------------------------------------------------------------|
| `Auth`            | Interface for validating authentication credentials                          |
| `AuthFunc`        | Function adapter for `Auth`                                                  |
| `ClientAuth`      | Interface for additional client-level authentication after initial auth       |
| `TokenExtractor`  | Interface for extracting tokens from context (default: Bearer from metadata) |
| `TokenProvider`   | Interface for providing tokens in outgoing client requests                    |
| `ScopeClientAuth` | `ClientAuth` adapter that enforces an `auth/scope.Enforcer`                   |
| `ScopeClientAuthPrincipal` / `WithScopeAudit` | `ScopeClientAuth` for `principal.Principal`; option recording each decision via `DecisionRecorder` |
| `ErrPrincipalTypeMismatch` | Wrapped when `Credentials.Data` is not the enforcer's principal type   |
| `Credentials`     | Authenticated credentials with headers and custom data                       |
| `CredentialsFromContext` | Reads the `Credentials` the server interceptor stores after authentication |
| `Request`         | Authentication request with method, token, and metadata                      |
| `ExtractBearerToken` / `ExtractTokenFromHeader` | Built-in `TokenExtractor` constructors                  |
| `StaticTokenProvider` | `TokenProvider` returning a fixed token for `ClientInterceptor`              |

After a successful authentication the server interceptor stores `Credentials` in the context (read them with `CredentialsFromContext`). When
`Credentials.Data` is an [`auth/principal`](../../../../auth/principal) `Principal` (or a non-nil `*Principal`), it is also stored through
`principal.NewContextFrom`; data that maps itself onto a principal, such as the `*oidc.Claims` of the [`oidc`](oidc) adapter, is stored as its
`Principal()`. A handler served over gRPC and HTTP reads the caller with `principal.FromContext`.

## Server options

| Option               | Default            | Description                                      |
|----------------------|--------------------|--------------------------------------------------|
| `WithAuthFn`         | --                 | Authentication function (required)                |
| `WithClientAuth`     | nil                | Additional client-level auth check                |
| `WithTokenExtractor` | Bearer token       | Custom token extraction logic                     |
| `WithIgnoreMethods`  | --                 | Exact full method names to skip authentication    |
| `WithIgnorePatterns` | --                 | Regex patterns for methods to skip                |
| `WithLogger`         | discard            | Structured logger                                 |

No method is exempt by default — gRPC health and reflection included. Exempt only what must be public, with exact full method names
(`WithIgnoreMethods("/grpc.health.v1.Health/Check")`) or patterns anchored at both ends (`(?i)^/grpc\.health\.v1\.Health/(Check|Watch)$`). The
reflection and health exemptions applied by default before are available as an explicit opt-in: `auth.WithIgnorePatterns(defaults.IgnorePatterns...)`
(package [`defaults`](../defaults)); note that its health pattern also matches any service whose name starts with `grpc.health.v1.Health`.

## Client options

| Option           | Default         | Description                                       |
|------------------|-----------------|---------------------------------------------------|
| `WithHeaderName` | "authorization" | Metadata header name for the token                |
| `WithScheme`     | "Bearer"        | Authentication scheme prefix (empty for none)     |

## Subpackages

| Package              | Description                                      |
|----------------------|--------------------------------------------------|
| [mtls](./mtls)       | Mutual-TLS peer certificate adapter over [`auth/mtls`](../../../../auth/mtls) |
| [oidc](./oidc)       | OIDC adapter over [`auth/oidc`](../../../../auth/oidc) `ClaimsValidator` |
| [static](./static)   | Static token/API key adapter over [`auth/static`](../../../../auth/static) |
