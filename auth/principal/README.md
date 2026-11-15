# principal

```go
import "github.com/altessa-s/go-atlas/auth/principal"
```

Canonical verified-identity type shared across the auth stack: a `Principal` carries the subject, tenant, granted scopes and roles, plus an escape
hatch
of raw claims. Authentication adapters stash one standard shape as a request's principal so services stop reinventing their own.

It is a plain data type with no decision logic — the generic authorization cores ([`auth/scope`](../scope)) stay parameterized over any `P` and read
what they need through extractors. `Principal` is simply a convenient, standard `P`.

## Type

| Field     | Type             | Description                                                       |
|-----------|------------------|-------------------------------------------------------------------|
| `Subject` | `string`         | Authenticated subject (token `sub` / certificate identity).       |
| `Tenant`  | `string`         | Organization/workspace scope; empty when single-tenant or absent. |
| `Scopes`  | `[]string`       | Granted authorization scopes (OAuth scope / RFC 9068).            |
| `Roles`   | `[]string`       | Granted roles, mapped to scopes by a role registry.               |
| `Claims`  | `map[string]any` | Raw claims not promoted to a field; may be nil.                   |

## Accessors

| Method                    | Description                                                |
|---------------------------|------------------------------------------------------------|
| `HasScope(s) bool`        | Whether scope `s` was granted.                             |
| `HasAnyScope(...s) bool`  | Whether at least one of the scopes was granted.            |
| `HasAllScopes(...s) bool` | Whether every scope was granted (vacuously true for none). |
| `HasRole(r) bool`         | Whether role `r` was granted.                              |
| `HasAnyRole(...r) bool`   | Whether at least one of the roles was granted.             |
| `Claim(name) (any, bool)` | Raw claim lookup.                                          |
| `IsZero() bool`           | Whether the principal carries no identity (anonymous).     |

## Building from claims

`FromClaims` maps a verified claim set through the narrow `ClaimsSource` interface (satisfied by [`auth/jwt`](../jwt) `Claims`), promoting
`sub`→`Subject`, the scope claim→`Scopes`, and the configurable tenant/roles claims.

| Option               | Description                                    |
|----------------------|------------------------------------------------|
| `WithTenantClaim(n)` | Claim read into `Tenant` (default `"tenant"`). |
| `WithRolesClaim(n)`  | Claim read into `Roles` (default `"roles"`).   |

```go
claims, _ := verifier.Verify(ctx, raw)
p := principal.FromClaims(claims, principal.WithRolesClaim("groups"))

if p.HasScope("files:read") { /* ... */ }
```

Auth adapters that map claims on every request should build a `Mapper` once — the option set is materialized a single time instead of per call:

```go
var mapper = principal.NewMapper(principal.WithRolesClaim("groups"))

p := mapper.FromClaims(claims)
```

The zero-value `Mapper` maps with the default claim names; a `Mapper` is immutable and safe for concurrent use.

## Wiring into the scope enforcer

`scope.Scope` is a string alias, so the slices plug straight in:

```go
enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
    func(p principal.Principal) []scope.Scope { return p.Scopes }, scope.Exact()))
```

## Request context

| Function                                 | Description                                                                              |
|------------------------------------------|------------------------------------------------------------------------------------------|
| `NewContext(ctx, p) ctx`                 | Returns a copy of `ctx` carrying `p` as the verified caller.                             |
| `NewContextFrom(ctx, v any) (ctx, bool)` | Stores `v` when it is a `Principal` (without boxing it again) or a non-nil `*Principal`. |
| `FromContext(ctx) (Principal, bool)`     | Reads the principal back; `false` when none is stored.                                   |

The HTTP auth middleware ([`transport/http/server/middlewares/auth`](../../transport/http/server/middlewares/auth)) and the gRPC auth interceptor
([`transport/grpc/interceptors/auth`](../../transport/grpc/interceptors/auth)) call `NewContextFrom` with their authenticator's result, so whenever it
is a `Principal` (or a non-nil `*Principal`) a handler served over both transports reads the caller the same way:

```go
if p, ok := principal.FromContext(ctx); ok && p.HasScope("files:read") {
    // ...
}
```

Authenticators that return an application-defined identity type (for example `spiffe.ID` from [`auth/mtls`](../mtls)) are not bridged; their
result stays reachable through the transport's own accessor (`auth.FromContext` over HTTP, `auth.CredentialsFromContext` over gRPC).
