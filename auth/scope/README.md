# scope

```go
import "github.com/altessa-s/go-atlas/auth/scope"
```

Package `scope` is a transport-neutral, deny-by-default authorization policy built on permission scopes. It answers one question —
may this principal perform this action? — with no knowledge of transports, I/O, or what a principal is, so the same policy drives
gRPC and HTTP authorization and is trivially unit-testable.

A `Registry` maps an action key (a gRPC full method, an HTTP route, any stable identifier) to the `Scope` it requires; it is built
once, frozen, then read concurrently. An `Enforcer` pairs that registry with an `Authorizer` — the single extension point that
decides whether a principal of the caller's own type `P` satisfies a required scope. Scope matching, superuser bypass, role or
tenant checks, and any extra principal fields are composed by the caller, so the core bakes in no identity fields.

## Key types

| Type            | Description                                                                                              |
|-----------------|---------------------------------------------------------------------------------------------------------|
| `Scope`         | `= string`. An opaque scope identifier, e.g. `"files:read"`.                                             |
| `Registry`      | Maps an action key to its required scope. Build, `Freeze`, then read concurrently. Deny-by-default.      |
| `Matcher`       | `func(granted []Scope, required Scope) bool` — pluggable comparison strategy.                            |
| `Authorizer[P]` | `func(p P, required Scope) bool` — the sole extension point; decides whether principal `P` is granted.   |
| `Enforcer[P]`   | Pairs a `Registry` with an `Authorizer[P]`; `Enforce` makes the deny-by-default decision.                |

## Functions

| Function                            | Description                                                                                  |
|-------------------------------------|----------------------------------------------------------------------------------------------|
| `NewRegistry()`                     | Empty registry ready for registration.                                                       |
| `Registry.Register(key, scope)`     | Record that `key` requires `scope` (empty scope = public). Panics after `Freeze`.            |
| `Registry.RegisterMany(scope, ...)` | Bulk-register many keys with the same scope.                                                  |
| `Registry.Freeze()`                 | Immutable snapshot, safe for lock-free concurrent reads; further registration panics.        |
| `Registry.Required(key)`            | `(scope, ok)`; `ok=false` (unregistered) is denied by default, `("", true)` is public.       |
| `Exact()`                           | `Matcher` of flat membership (`slices.Contains`). The default strategy.                       |
| `Wildcard(sep)`                     | `Matcher` honoring hierarchical grants: `"*"` and `"files:*"` (with `sep` `":"`) cover more.  |
| `ScopeAuthorizer(scopesOf, m)`      | Build the common `Authorizer`: `p` granted ⇔ its scopes satisfy `required` under matcher `m`. |
| `AnyOf(authorizers…)`               | Compose authorizers with OR — granted if any is (empty denies). E.g. scope **or** superuser.  |
| `AllOf(authorizers…)`               | Compose authorizers with AND — granted only if all are (empty grants). E.g. scope **and** gate. |
| `NewEnforcer(reg, authorize)`       | Enforcer backed by `reg`, delegating the satisfy decision to `authorize`.                     |
| `Enforcer.Enforce(p, key)`          | `nil` if allowed, else `ErrAccessDenied`.                                                     |

## Decision

`Enforce` is fail-closed:

| Case                              | Result                       |
|-----------------------------------|------------------------------|
| Unregistered key                  | `ErrAccessDenied`            |
| Key registered with empty scope   | allowed (public)             |
| Otherwise                         | the `Authorizer` decides     |

`ErrAccessDenied` is transport-neutral; adapters map it (gRPC `codes.PermissionDenied`, HTTP `403`).

## Usage

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

## Object-level authorization

When a decision depends on the specific resource being acted on (ownership, per-object ACLs), use the resource-aware variants. They
mirror the action-level API with one extra resource parameter `R`, the same deny-by-default registry, and the same fail-closed
decision (unregistered key denied, empty scope public and resource not consulted).

| Type / Function                       | Description                                                                                       |
|---------------------------------------|---------------------------------------------------------------------------------------------------|
| `ResourceAuthorizer[P, R]`            | `func(p P, r R, required Scope) bool` — object-level extension point (scope + ownership + tenant). |
| `LiftAuthorizer[P, R](a)`             | Adapt an `Authorizer[P]` to a `ResourceAuthorizer[P, R]` that ignores the resource.                |
| `ResourceAnyOf` / `ResourceAllOf`     | Compose resource authorizers with OR / AND (same identities as `AnyOf` / `AllOf`).                 |
| `NewResourceEnforcer(reg, authorize)` | `ResourceEnforcer[P, R]` backed by `reg`.                                                          |
| `ResourceEnforcer.Enforce(p, r, key)` | `nil` if allowed, else `ErrAccessDenied`.                                                          |

```go
base := scope.ScopeAuthorizer(func(p *Principal) []scope.Scope { return p.Scopes }, scope.Exact())
scoped := scope.LiftAuthorizer[*Principal, *Doc](base)
owns := func(p *Principal, d *Doc, _ scope.Scope) bool { return d.OwnerID == p.ID }

// "has the scope AND owns the resource"; OR-in a superuser bypass if needed.
enf := scope.NewResourceEnforcer(reg, scope.ResourceAllOf(scoped, owns))
if err := enf.Enforce(p, doc, "/docs.v1.Docs/Update"); err != nil {
    // errors.Is(err, scope.ErrAccessDenied)
}
```

## Role-based access

Services that model access as roles rather than raw scopes can map roles to scopes with `RoleScopes` instead of hand-writing the
expansion. The core stays role-agnostic: the role→scope table is the caller's policy, and a caller-supplied `rolesOf` extractor reads
roles off the principal, so no role field enters the core. `RoleAuthorizer` composes the table and extractor into a plain `Authorizer`.

| Type / Function                       | Description                                                                                       |
|---------------------------------------|---------------------------------------------------------------------------------------------------|
| `RoleScopes`                          | Immutable role→scopes table built once with `NewRoleScopes`, safe for concurrent reads.            |
| `NewRoleScopes(mapping)`              | Build the table from `map[string][]Scope`; scope slices are cloned. Nil/empty grants nothing.      |
| `RoleScopes.ScopesFor(roles…)`        | Union of the roles' scopes, first-seen order, deduplicated. Unknown roles and nil receiver → nil.  |
| `RoleScopesOf[P](rolesOf, rs)`        | Adapt a `rolesOf func(P) []string` into the `scopesOf` that `ScopeAuthorizer` expects.             |
| `RoleAuthorizer[P](rolesOf, rs, m)`   | Convenience for `ScopeAuthorizer(RoleScopesOf(rolesOf, rs), m)`.                                   |

```go
rs := scope.NewRoleScopes(map[string][]scope.Scope{
    "viewer": {"files:read"},
    "editor": {"files:read", "files:write"},
    "admin":  {"files:read", "files:write", "users:manage"},
})

// rolesOf reads roles off the caller's own principal — the core never names a role field.
authorize := scope.RoleAuthorizer(func(p *Principal) []string { return p.Roles }, rs, scope.Exact())

// Add a superuser bypass exactly as with a plain scope authorizer:
authorize = scope.AnyOf(authorize, func(p *Principal, _ scope.Scope) bool { return p.Superuser })

enf := scope.NewEnforcer(reg, authorize)
```

## Transports

This is a policy primitive only. The gRPC adapter `ScopeClientAuth` in
[`transport/grpc/interceptors/auth`](../../transport/grpc/interceptors/auth) drives an `Enforcer` from the verified principal in
`Credentials.Data`, keyed by the full method name. An HTTP adapter (keyed by route) is left to the transport layer.
