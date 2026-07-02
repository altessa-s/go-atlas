# Canonical Principal

The one verified-identity type the auth stack agrees on. Authentication adapters build a `Principal` — subject, tenant, granted scopes and
roles, plus an escape hatch of raw claims — and stash it as the request's principal, so a service reads identity through one shape instead of
reinventing its own per transport.

---

## Table of Contents

- [Overview](#overview)
- [The Type](#the-type)
- [Accessors](#accessors)
- [Building from Claims](#building-from-claims)
- [Relationship to the Decision Cores](#relationship-to-the-decision-cores)
- [Quick Start](#quick-start)
  - [From verified claims](#from-verified-claims)
  - [Driving the scope enforcer](#driving-the-scope-enforcer)
- [API Reference](#api-reference)
- [Design Notes](#design-notes)
- [See Also](#see-also)

## Overview

`auth/principal` is a plain data type with no decision logic. It carries what a request needs to know about its caller after
authentication, in fields every service understands. It exists so the AuthN adapters (`oidc`, `mtls`, `selfjwt`, `static`) can produce a
single standard value and the AuthZ cores can consume it — without either side inventing a bespoke identity struct.

```go
import "github.com/altessa-s/go-atlas/auth/principal"
```

It is deliberately not an interface and not a decision core: the generic authorizers stay parameterized over any `P` and read what they need
through caller-supplied extractors. `Principal` is just a convenient, standard `P`.

## The Type

| Field     | Type             | Role                                                                |
|-----------|------------------|---------------------------------------------------------------------|
| `Subject` | `string`         | Authenticated subject (token `sub` / certificate identity). Empty = anonymous. |
| `Tenant`  | `string`         | Organization/workspace scope; empty when single-tenant or absent.   |
| `Scopes`  | `[]string`       | Granted authorization scopes (OAuth `scope`, RFC 9068).             |
| `Roles`   | `[]string`       | Granted roles, mapped to scopes by a role registry where role-based. |
| `Claims`  | `map[string]any` | Raw claims not promoted to a field, for service-specific needs; may be nil. |

Every field is optional. The zero `Principal` is a valid anonymous identity, and `IsZero` reports it.

## Accessors

The common membership checks live as methods so callers don't re-scan the slices by hand.

| Method                    | Result                                                     |
|---------------------------|------------------------------------------------------------|
| `HasScope(s)`             | Whether scope `s` was granted.                             |
| `HasAnyScope(s…)`         | Whether at least one of the scopes was granted (none → false). |
| `HasAllScopes(s…)`        | Whether every scope was granted (none → true, vacuously).  |
| `HasRole(r)`              | Whether role `r` was granted.                              |
| `HasAnyRole(r…)`          | Whether at least one of the roles was granted (none → false). |
| `Claim(name)`             | Raw claim lookup — `(value, ok)`; `ok=false` when `Claims` is nil or absent. |
| `IsZero()`                | Whether the principal carries no identity at all.          |

## Building from Claims

`FromClaims` maps a verified claim set onto a `Principal`, reading through the narrow `ClaimsSource` interface rather than a concrete token
type. It promotes `sub`→`Subject`, the scope claim→`Scopes`, and the configurable tenant/roles claims. The raw `Claims` map is left nil —
set it on the result when a service needs claims beyond the promoted fields.

| Claim source method        | Reads                                                    |
|----------------------------|----------------------------------------------------------|
| `Subject() string`         | the `sub` claim → `Subject`.                             |
| `Scopes() []string`        | the granted scopes → `Scopes`.                           |
| `String(name)`             | the tenant claim → `Tenant`.                             |
| `StringSlice(name)`        | the roles claim → `Roles`.                               |

`ClaimsSource` is satisfied by [`auth/jwt`](../../auth/jwt) `Claims` and any other claim type exposing the same accessors, so the mapping
stays decoupled from a particular token package.

| Option               | Effect                                            |
|----------------------|---------------------------------------------------|
| `WithTenantClaim(n)` | Claim read into `Tenant` (default `"tenant"`).    |
| `WithRolesClaim(n)`  | Claim read into `Roles` (default `"roles"`).      |

## Relationship to the Decision Cores

`Principal` does not change how authorization works — [`auth/scope`](scope.md) stays generic over the caller's `P`. It just saves you the
boilerplate: because `scope.Scope` is a string alias, a principal's `Scopes` and `Roles` plug straight into `ScopeAuthorizer` /
`RoleAuthorizer` through a one-line extractor, and any composite rule (superuser, tenant match) is layered by the caller exactly as with a
custom principal type.

## Quick Start

### From verified claims

```go
import "github.com/altessa-s/go-atlas/auth/principal"

claims, err := verifier.Verify(ctx, raw) // any principal.ClaimsSource, e.g. auth/jwt Claims
if err != nil {
    return err
}
p := principal.FromClaims(claims, principal.WithRolesClaim("groups"))

if p.HasScope("files:read") {
    // ...
}
```

### Driving the scope enforcer

```go
import (
    "github.com/altessa-s/go-atlas/auth/principal"
    "github.com/altessa-s/go-atlas/auth/scope"
)

// scope.Scope is a string alias, so Scopes plug straight in.
enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
    func(p principal.Principal) []scope.Scope { return p.Scopes }, scope.Exact()))

// Compose a superuser bypass without the core ever naming such a field:
authorize := scope.AnyOf(
    scope.ScopeAuthorizer(func(p principal.Principal) []scope.Scope { return p.Scopes }, scope.Exact()),
    func(p principal.Principal, _ scope.Scope) bool { return p.HasRole("admin") },
)
enf = scope.NewEnforcer(reg, authorize)
```

## API Reference

| Symbol                               | Description                                                                              |
|--------------------------------------|------------------------------------------------------------------------------------------|
| `principal.Principal`                | Canonical verified identity (`Subject`/`Tenant`/`Scopes`/`Roles`/`Claims`).              |
| `Principal.HasScope(s)`              | Scope membership check.                                                                   |
| `Principal.HasAnyScope(s…)` / `HasAllScopes(s…)` | Any/all scope checks (none → false / true).                                   |
| `Principal.HasRole(r)` / `HasAnyRole(r…)` | Role membership checks.                                                              |
| `Principal.Claim(name)`              | Raw claim lookup, `(value, ok)`.                                                          |
| `Principal.IsZero()`                 | Whether the principal is anonymous (no identity at all).                                  |
| `principal.ClaimsSource`             | Narrow claim-set view `FromClaims` reads; satisfied by `auth/jwt` `Claims`.               |
| `principal.FromClaims(c, opt…)`      | Build a `Principal` from a verified claim set.                                            |
| `principal.WithTenantClaim(n)`       | Name of the claim mapped to `Tenant` (default `"tenant"`).                                |
| `principal.WithRolesClaim(n)`        | Name of the claim mapped to `Roles` (default `"roles"`).                                  |

## Design Notes

- **Data, not decision.** No package logic decides access from a `Principal`; that stays in the caller's `Authorizer`. This keeps the type
  reusable across services with different policies. See [`../../auth/principal/README.md`](../../auth/principal/README.md).
- **Standard `P`, not a mandatory one.** The decision cores never require `Principal` — they stay generic. Adopt it to stop hand-rolling an
  identity struct; keep a custom `P` when a service genuinely needs one.
- **Claim mapping is decoupled.** `FromClaims` reads through `ClaimsSource`, so it works with any verifier's claim type, not just one token
  package.

## See Also

- [`auth/principal` README](../../auth/principal/README.md) — package quick reference.
- [scope.md](scope.md) — the authorizer that consumes a principal's scopes and roles.
- [oidc.md](oidc.md) · [selfjwt.md](selfjwt.md) — AuthN mechanisms whose claims feed `FromClaims`.
- [architecture.md](../architecture.md) — package map and layering.
