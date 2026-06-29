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

## Transports

This is a policy primitive only. The gRPC adapter `ScopeClientAuth` in
[`transport/grpc/interceptors/auth`](../../transport/grpc/interceptors/auth) drives an `Enforcer` from the verified principal in
`Credentials.Data`, keyed by the full method name. An HTTP adapter (keyed by route) is left to the transport layer.
