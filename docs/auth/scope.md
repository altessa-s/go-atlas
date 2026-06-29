# Scope-Based Authorization

Transport-neutral, deny-by-default authorization built on permission scopes. One question — may this principal perform this action? — answered
with no knowledge of transports, I/O, or what a principal is, so the same policy drives gRPC and HTTP authorization and is trivially unit-testable.

---

## Table of Contents

- [Overview](#overview)
- [Model](#model)
- [Decision](#decision)
- [Quick Start](#quick-start)
  - [Core policy](#core-policy)
  - [gRPC](#grpc)
  - [HTTP](#http)
  - [OIDC claims](#oidc-claims)
  - [Roles](#roles)
  - [Config-driven registry](#config-driven-registry)
- [API Reference](#api-reference)
- [Design Notes](#design-notes)
- [See Also](#see-also)

## Overview

The `auth/scope` package is a pure policy primitive: a `Registry` mapping action keys to required scopes, an `Enforcer` that decides access, and a
pluggable `Matcher` for how granted scopes satisfy a requirement. It owns no identity type — the caller's principal stays the caller's, injected through a
single `Authorizer` function. Transport adapters (gRPC interceptor seam, HTTP middleware) wrap the same `Enforcer`.

```go
import "github.com/altessa-s/go-atlas/auth/scope"
```

## Model

| Concept         | Type                                       | Role                                                                                       |
|-----------------|--------------------------------------------|--------------------------------------------------------------------------------------------|
| Scope           | `scope.Scope` (`= string`)                 | Opaque scope identifier, e.g. `"files:read"`.                                               |
| Registry        | `*scope.Registry`                          | Maps an action key (gRPC full method, HTTP route, …) to its required scope. Deny-by-default. |
| Matcher         | `scope.Matcher`                            | Decides whether granted scopes satisfy a required one. `Exact` (default) or `Wildcard`.     |
| Authorizer      | `scope.Authorizer[P]`                      | The sole extension point — `func(p P, required Scope) bool`. Holds all principal knowledge.  |
| Enforcer        | `*scope.Enforcer[P]`                       | Pairs a `Registry` with an `Authorizer[P]`; `Enforce` makes the decision.                    |

A `Registry` is built once, frozen, then read concurrently without locks. Scope matching, superuser bypass, role or tenant checks, and any extra
principal fields are composed by the caller inside the `Authorizer`, so the core bakes in no identity fields.

## Decision

`Enforcer.Enforce(p, key)` is fail-closed:

| Case                              | Result                       |
|-----------------------------------|------------------------------|
| Unregistered key                  | `scope.ErrAccessDenied`      |
| Key registered with empty scope   | allowed (intentionally public) |
| Otherwise                         | the `Authorizer` decides     |

`ErrAccessDenied` is transport-neutral; adapters map it (gRPC `codes.PermissionDenied`, HTTP `403`). A superuser bypass composed into the
`Authorizer` does **not** reach unregistered keys: deny-by-default runs before the `Authorizer`, so a brand-new unmapped action is denied to everyone.

## Quick Start

### Core policy

```go
type Principal struct {
    Scopes    []string
    Superuser bool
}

reg := scope.NewRegistry()
reg.Register("/files.v1.Files/Read", "files:read")
reg.Register("/files.v1.Files/Write", "files:write")
reg.Register("/health.v1.Health/Check", "") // public
reg.Freeze()

base := scope.ScopeAuthorizer(func(p *Principal) []string { return p.Scopes }, scope.Exact())
authorize := func(p *Principal, required scope.Scope) bool {
    return p.Superuser || base(p, required) // superuser is the caller's concept, not the core's
}
enf := scope.NewEnforcer(reg, authorize)

if err := enf.Enforce(p, "/files.v1.Files/Write"); err != nil {
    // errors.Is(err, scope.ErrAccessDenied)
}
```

### gRPC

`ScopeClientAuth` adapts an `Enforcer` to the auth interceptor's `ClientAuth` seam. The verified principal is read from `Credentials.Data`
(the value the `Auth` function returns); the action key is `Credentials.FullyMethodName`. Denial surfaces as `codes.PermissionDenied`.

```go
import grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

interceptor := grpcauth.ServerInterceptor(
    grpcauth.WithAuthFunc(authFunc),                 // returns *Principal as Credentials.Data
    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf)),
)
```

### HTTP

`ScopeMiddleware` enforces an `Enforcer` over authenticated requests. The principal is read from the request context via `auth.FromContext` (set by
the authentication middleware); `keyFunc` maps the request to the registered action key — typically the matched route pattern. Denial responds `403`.

```go
import httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"

keyFunc := func(r *http.Request) string { return r.Method + " " + r.Pattern }
mw := httpauth.ScopeMiddleware(enf, keyFunc) // run after the authentication middleware
```

### OIDC claims

When the principal is an OIDC `*Claims`, `oidc.ScopesOf` is a ready-made `scopesOf` for `ScopeAuthorizer`:

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"

enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(oidc.ScopesOf, scope.Exact()))
```

### Roles

When a service models access as roles rather than raw scopes, `RoleScopes` maps roles to scopes so it can still drive the scope `Enforcer`.
The core stays role-agnostic: the role→scope table is the caller's policy, and a caller-supplied `rolesOf` extractor reads roles off the
principal — no role or identity field enters the core. `RoleAuthorizer` is the thin composite over `ScopeAuthorizer` + `RoleScopesOf`.

```go
type Principal struct {
    Roles     []string
    Superuser bool
}

rs := scope.NewRoleScopes(map[string][]scope.Scope{
    "viewer": {"files:read"},
    "editor": {"files:read", "files:write"},
    "admin":  {"files:read", "files:write", "users:manage"},
})

// rolesOf reads roles off the caller's own principal; the core never names a role field.
authorize := scope.RoleAuthorizer(func(p *Principal) []string { return p.Roles }, rs, scope.Exact())

// Add a superuser bypass exactly as with a plain scope authorizer:
authorize = scope.AnyOf(authorize, func(p *Principal, _ scope.Scope) bool { return p.Superuser })

enf := scope.NewEnforcer(reg, authorize)
```

`RoleScopes.ScopesFor(roles…)` returns the deduplicated union of the roles' scopes (unknown roles contribute nothing), and
`RoleScopesOf(rolesOf, rs)` is the `scopesOf` adapter if you prefer to build the authorizer through `ScopeAuthorizer` yourself.

### Config-driven registry

The registry can be built from configuration instead of by hand, symmetric with the OPA factory. `auth/scope/factory.New(cfg).Build()`
registers the rules in a `config.ScopeRegistry` and returns a frozen registry. Only the key→scope table is declarative; the matcher and
authorizer stay in code, since they depend on the principal type.

```go
import scopefactory "github.com/altessa-s/go-atlas/auth/scope/factory"

reg, err := scopefactory.New(&cfg.Scope).Build() // cfg.Scope is a config.ScopeRegistry
if err != nil {
    return err
}
enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(scopesOf, scope.Exact()))
```

```yaml
scope:
  rules:
    - scope: "files:read"
      keys: ["/files.v1.Files/Read", "/files.v1.Files/List"]
    - scope: ""               # public
      keys: ["/health.v1.Health/Check"]
```

`Build` rejects a nil config, a rule with no keys, or an action key that appears in more than one rule.

## API Reference

| Symbol                                | Description                                                                                   |
|---------------------------------------|-----------------------------------------------------------------------------------------------|
| `NewRegistry()`                       | Empty registry ready for registration.                                                        |
| `Registry.Register(key, scope)`       | Record that `key` requires `scope` (empty scope = public). Panics after `Freeze`.            |
| `Registry.RegisterMany(scope, keys…)` | Bulk-register many keys with the same scope.                                                  |
| `Registry.Freeze()`                   | Immutable snapshot, safe for lock-free concurrent reads; further registration panics.         |
| `Registry.Required(key)`              | `(scope, ok)`; `ok=false` (unregistered) is denied by default, `("", true)` is public.        |
| `Exact()`                             | `Matcher` of flat membership. The default strategy.                                            |
| `Wildcard(sep)`                       | `Matcher` honoring hierarchical grants: `"*"` and `"files:*"` (with `sep` `":"`) cover more.   |
| `ScopeAuthorizer(scopesOf, m)`        | Build the common `Authorizer` from a scope extractor and a matcher.                            |
| `NewRoleScopes(mapping)`              | Immutable role→scopes table from `map[string][]Scope`; scope slices are cloned.                |
| `RoleScopes.ScopesFor(roles…)`        | Deduplicated union of the roles' scopes; unknown roles and a nil receiver yield nil.           |
| `RoleScopesOf(rolesOf, rs)`           | Adapt a `rolesOf func(P) []string` into the `scopesOf` that `ScopeAuthorizer` expects.         |
| `RoleAuthorizer(rolesOf, rs, m)`      | Convenience for `ScopeAuthorizer(RoleScopesOf(rolesOf, rs), m)` — roles stay caller-side.       |
| `AnyOf(authorizers…)`                 | Compose authorizers with OR — granted if any is (empty denies). E.g. scope **or** superuser.    |
| `AllOf(authorizers…)`                 | Compose authorizers with AND — granted only if all are (empty grants). E.g. scope **and** gate. |
| `NewEnforcer(reg, authorize)`         | Enforcer backed by `reg`, delegating the satisfy decision to `authorize`.                      |
| `Enforcer.Enforce(p, key)`            | `nil` if allowed, else `ErrAccessDenied`.                                                      |
| `grpcauth.ScopeClientAuth(enf)`       | gRPC `ClientAuth` adapter (key = full method, principal = `Credentials.Data`).                 |
| `httpauth.ScopeMiddleware(enf, keyFn)`| HTTP middleware (principal = `auth.FromContext`, key = `keyFn(r)`, 403 on denial).             |
| `oidc.ScopesOf(*Claims)`              | `scopesOf` adapter returning an OIDC token's granted scopes.                                   |

## Design Notes

- **No baked-in identity fields.** The core never knows about superusers, tenants, or roles — they live in the caller's `Authorizer`, so one package serves
  every service shape. See the package `README.md` for the rationale.
- **Generics, not interfaces.** `Enforcer[P]` is parameterized over the caller's principal type `P`; no interface is imposed and no type assertion happens in
  the core (the gRPC adapter performs the single `Credentials.Data.(P)` assertion at the transport boundary).
- **Freeze pattern.** `Registry` mirrors the build-once / read-many `coremaps.ImmutableMap` convention used across the repo.

## See Also

- [`auth/scope` README](../../auth/scope/README.md) — package quick reference.
- [oidc.md](oidc.md) — OIDC/JWT validation that produces the `Claims` used as a principal.
- [architecture.md](../architecture.md) — package map and layering.
