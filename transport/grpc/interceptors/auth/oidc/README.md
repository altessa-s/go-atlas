# oidc

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
```

Package `oidc` provides OIDC (OpenID Connect) token validation for the auth interceptor. Bridges the auth interceptor with OIDC providers via the
`Validator` interface. Provides `AuthFunc` for integration with gRPC auth, structured `Claims` extraction, and ScopeRegistry compatibility.

## Key types

| Type / Interface | Description                                                                       |
|------------------|-----------------------------------------------------------------------------------|
| `Validator`      | Interface for OIDC token validation: `ValidateToken(ctx, token) (map, error)`     |
| `AuthFunc`       | Creates an `auth.AuthFunc` from a Validator for gRPC auth interceptor integration |
| `Claims`         | Structured OIDC claims: Subject, Email, Scopes, Issuer, Audience, and timestamps  |

## Claims fields

| Field               | JSON Key             | Description                                       |
|---------------------|----------------------|---------------------------------------------------|
| `Subject`           | `sub`                | Unique user identifier within the issuer          |
| `PreferredUsername`  | `preferred_username` | Display username (not guaranteed unique)           |
| `Email`             | `email`              | User email address                                |
| `Issuer`            | `iss`                | OIDC provider that issued the token               |
| `Audience`          | `aud`                | Intended token recipients (string or array)        |
| `Scopes`            | `scope`              | Granted OAuth 2.0 scopes (sorted alphabetically)  |
| `ExpiresAt`         | `exp`                | Token expiration time                              |
| `IssuedAt`          | `iat`                | Token issuance time                                |
| `NotBefore`         | `nbf`                | Token validity start time                          |
| `RawClaims`         | --                   | All claims from the original token (custom access) |

## Subpackages

| Package                    | Description                                  |
|----------------------------|----------------------------------------------|
| [validator](./validator)   | Production-ready OIDC token validator         |
