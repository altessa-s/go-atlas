# Authentication & Authorization

The `auth/` packages handle two jobs: proving who a caller is (authentication) and deciding what they may do (authorization), plus a durable
trail of those decisions. There is no framework to bootstrap. Each package works on its own, takes its configuration through functional
options, falls back to a safe no-op when left unset, and plugs into the gRPC and HTTP transports through small adapters.

This document is the map; for depth on any one mechanism, follow the per-topic guides linked throughout.

---

## Table of Contents

- [Authentication vs Authorization](#authentication-vs-authorization)
- [Package Map](#package-map)
- [The Request Pipeline](#the-request-pipeline)
- [Initialization](#initialization)
  - [Direct construction](#direct-construction)
  - [Factory + config template](#factory--config-template)
- [Transport Wiring](#transport-wiring)
  - [gRPC](#grpc)
  - [HTTP](#http)
- [Recipes](#recipes)
  - [Static API-key authentication (HTTP)](#static-api-key-authentication-http)
  - [OIDC token validation](#oidc-token-validation)
  - [Self-issued JWTs between services](#self-issued-jwts-between-services)
  - [Scope policy over OIDC claims](#scope-policy-over-oidc-claims)
  - [OPA policy with audit](#opa-policy-with-audit)
  - [Full gRPC server: OIDC + scope + audit](#full-grpc-server-oidc--scope--audit)
- [Cross-Cutting Concerns](#cross-cutting-concerns)
- [Design Principles](#design-principles)
- [See Also](#see-also)

## Authentication vs Authorization

The two halves answer different questions. An authenticator proves identity and yields a principal; an authorizer takes that principal and
decides access. Because they stay separate, the same OIDC token can feed either an OPA policy or a scope check, and a scope policy can be
unit-tested with a fake principal.

| Question                          | Layer    | Packages                                  |
|-----------------------------------|----------|-------------------------------------------|
| Who is the caller?                | AuthN    | `jwt`, [`oidc`](oidc.md), [`selfjwt`](selfjwt.md), [`static`](static.md) |
| May this caller do this action?   | AuthZ    | [`scope`](scope.md), [`opa`](opa.md)      |
| What was decided, and why?        | Audit    | [`audit`](audit.md)                       |

AuthN produces a *principal* (an OIDC `*Claims`, a static token's associated data, …). AuthZ consumes that principal and owns no identity type
of its own. Audit records either layer's outcome.

## Package Map

| Package                     | Role        | Summary                                                                                       | Guide |
|-----------------------------|-------------|-----------------------------------------------------------------------------------------------|-------|
| `auth/jwt`                  | AuthN core  | Low-level JWT `Signer`/`Verifier`/`Claims` over golang-jwt; the shared core for selfjwt and oidc. | — |
| `auth/oidc`                 | AuthN       | OIDC/JWT validation with JWKS auto-rotation, CEL claim rules, introspection, presets.          | [oidc.md](oidc.md) |
| `auth/selfjwt`              | AuthN       | Self-issued JWT minting + verification with per-subject keys and rotation.                     | [selfjwt.md](selfjwt.md) |
| `auth/static`               | AuthN       | Static token / API-key validation for service-to-service calls, with optional rate limiting.   | [static.md](static.md) |
| `auth/scope`                | AuthZ       | Transport-neutral, deny-by-default scope policy (`Registry` + `Enforcer` + `Matcher`).         | [scope.md](scope.md) |
| `auth/opa`                  | AuthZ       | Open Policy Agent Rego evaluation with policy hot-reload and event-driven reload.              | [opa.md](opa.md) |
| `auth/audit`               | Audit       | `Decision` + consumer-side `Sink` seam + `Recorder` with policy/failure modes.                | [audit.md](audit.md) |
| `auth/audit/sinks/dataaudit` | Audit sink | Bridges `audit.Sink` to the framework's `data/audit` dispatcher.                               | [audit.md](audit.md) |

## The Request Pipeline

A protected request flows through up to three stages. Each is optional and independently configured; a service may run only AuthN, only
AuthZ, or both, with audit attached to either.

```
request ──▶ AuthN ──────────▶ AuthZ ─────────────▶ handler
            (oidc/static/      (scope/opa decide       │
             selfjwt verify     on the principal)       │
             token → principal)        │               │
                  │                     │               │
                  └────────┬───────────┘               │
                           ▼                            │
                        audit.Recorder ◀────────────────┘
                   (records the decision via a Sink)
```

- **AuthN** turns a credential into a principal and attaches it to the call (gRPC `Credentials.Data`, HTTP request context).
- **AuthZ** reads that principal and the action key, and allows or denies — fail-closed, deny-by-default.
- **Audit** records the outcome of either layer through a `Recorder`; opt-in and non-breaking.

## Initialization

Every package offers two construction paths. Use **direct construction** for full control in code; use the **factory + config template**
when configuration is loaded from YAML/env through `config/loader`.

### Direct construction

| Package    | Primary constructor                                                                 | Notes |
|------------|-------------------------------------------------------------------------------------|-------|
| `jwt`      | `NewSigner(opts…) *Signer` / `NewVerifier(resolver KeyResolver, opts…) *Verifier`   | Mandatory `KeyResolver` is positional. |
| `oidc`     | `NewProvider(ctx, discoveryURL, opt…) (*Provider, error)`                           | Starts JWKS refresh; default validation leeway `30s`. |
| `selfjwt`  | `New(src KeyProvider, opts…) (*Minter, *Verifier)`                                  | Returns a matched minter/verifier pair; `NewMinter`/`NewVerifier` for either alone. |
| `static`   | `NewInMemoryStore(opt…) *InMemoryStore`                                             | Wrap with `NewRateLimitedStore(store, limiter, keyFn)` to gate attempts. |
| `scope`    | `NewRegistry()` → `Register`/`Freeze`, then `NewEnforcer(reg, authorize)`           | `authorize` is the sole principal seam; registry is build-once / read-many. |
| `opa`      | `NewManager(ctx, source, query, opts…) (*Manager, error)`                           | `source` is the policy source (hot-reloadable); `query` is the Rego entrypoint. |
| `audit`    | `NewRecorder(sink, opt…) *Recorder`                                                 | Nil sink → no-op; wrap a `dataaudit.New(auditor)` sink for durability. |

### Factory + config template

Two packages ship a `factory/` builder that constructs the component from a config struct; the rest are configured directly in code.

| Package | Factory entrypoint                       | Config struct (Go)        | YAML template                       |
|---------|------------------------------------------|---------------------------|-------------------------------------|
| `oidc`  | `oidc/factory.New(cfg *config.OIDC) *ProviderBuilder` | `config.OIDC` (`config/auth_oidc.go`) | `config/templates/auth_oidc.yaml` |
| `opa`   | `opa/factory.New(cfg *config.OPA) *ManagerBuilder`    | `config.OPA` (`config/opa.go`)        | — (configured via `config.OPA`)   |

The builder pattern resolves dependencies and applies options; see each package's `factory/` README for the `Build`/accessor surface. The
`config/templates/auth.yaml` template is the aggregate auth section consumed by `config/loader`.

## Transport Wiring

AuthN and AuthZ plug into the transports through two distinct seams: an **`Auth`** function (authenticate → principal) and a **`ClientAuth`**
/ middleware (authorize the principal). The same `scope.Enforcer` or `opa.Manager` backs both gRPC and HTTP.

### gRPC

`transport/grpc/interceptors/auth`:

```go
import (
    grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
    grpcoidc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
)

interceptor := grpcauth.ServerInterceptor(
    grpcauth.WithAuthFn(grpcoidc.AuthFunc(validator)),         // AuthN: token → *Claims (Credentials.Data)
    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf,      // AuthZ: scope decision on that principal
        grpcauth.WithScopeAudit(rec, subjectOf))),            // optional audit
    grpcauth.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
)
```

Symbols: `ServerInterceptor(opt…)`, `WithAuthFn(Auth)`, `WithClientAuth(ClientAuth)`, `WithTokenExtractor`, `WithIgnoreMethods` /
`WithIgnorePatterns`; `ScopeClientAuth[P](e, opts…)` + `WithScopeAudit[P]`; per-mechanism `static.AuthFunc(store, opts…)` and
`oidc.AuthFunc(validator, opts…)`, each with a `WithAudit` option.

### HTTP

`transport/http/server/middlewares/auth`:

```go
import httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"

authn := httpauth.Middleware(/* AuthN options → sets principal in context */)
authz := httpauth.ScopeMiddleware(enf,
    func(r *http.Request) string { return r.Method + " " + r.Pattern },
    httpauth.WithScopeAudit(rec, subjectOf))

// chain: authn first (populates context), then authz
handler = authn(authz(handler))
```

Symbols: `Middleware(opt…)`, `FromContext(ctx)` (reads the principal), `ScopeMiddleware[P](e, keyFunc, opts…)` + `WithScopeAudit[P]`;
`static.AuthFunc(store, opts…)` + `WithAudit`. The authentication middleware must run before the scope middleware that reads the principal.

## Recipes

Self-contained snippets for the common shapes. Each one compiles against the signatures listed above; imports are abbreviated where the
package is obvious.

### Static API-key authentication (HTTP)

Seed a store with known keys and gate a handler. The store maps each token to whatever value you want as the principal.

```go
import (
    "github.com/altessa-s/go-atlas/auth/static"
    httpstatic "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/static"
    httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
)

type apiClient struct{ ID, Tier string }

store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
    "sk_live_abc123": apiClient{ID: "acme", Tier: "pro"},
}))

// Optional: gate validation attempts with a rate limiter.
// store = static.NewRateLimitedStore(store, limiter, keyFn)

authn := httpauth.Middleware(httpauth.WithAuthFunc(httpstatic.AuthFunc(store)))
mux.Handle("GET /v1/things", authn(thingsHandler))

// Inside the handler, recover the principal:
func thingsHandler(w http.ResponseWriter, r *http.Request) {
    client, _ := httpauth.FromContext(r.Context()).(apiClient)
    _ = client.Tier
}
```

A direct, transport-free validation is just `store.Validate(ctx, token) (any, error)`; the middleware wraps that and maps
`static.ErrTokenInvalid` / `ErrTokenEmpty` / `ErrRateLimited` to `401` / `429`.

### OIDC token validation

`Provider` discovers the issuer's metadata and JWKS at construction and keeps the keys fresh. `ValidateToken` returns the verified claims.

```go
import "github.com/altessa-s/go-atlas/auth/oidc"

provider, err := oidc.NewProvider(ctx, "https://issuer.example.com/.well-known/openid-configuration")
if err != nil {
    return err
}
claims, err := provider.ValidateToken(ctx, rawJWT) // (map[string]any, error); default leeway 30s
if err != nil {
    // errors.Is(err, oidc.ErrTokenInvalid) / oidc.ErrTokenRevoked
    return err
}
sub, _ := claims["sub"].(string)
```

Loading the provider from a `config.OIDC` instead (the factory path, e.g. when config comes from `config/templates/auth_oidc.yaml`):

```go
import (
    "github.com/altessa-s/go-atlas/auth/oidc/factory"
    "github.com/altessa-s/go-atlas/config"
)

provider, err := factory.New(&cfg /* *config.OIDC */).Build(ctx)
```

Per-call overrides (extra audience, required claims) go through `provider.ValidateTokenWithOptions(ctx, raw, opt…)`; see [oidc.md](oidc.md).

### Self-issued JWTs between services

The issuing service mints short-lived tokens; the receiving service verifies them. `New` returns a matched pair backed by the same key
provider.

```go
import "github.com/altessa-s/go-atlas/auth/selfjwt"

minter, verifier := selfjwt.New(keyProvider)

// Issuer side:
res, err := minter.Mint(ctx, selfjwt.MintRequest{
    Subject: "svc-billing",
    Scopes:  []string{"ledger:write"},
    TTL:     5 * time.Minute,
})
// res.Token is the JWT; persist res.ID (jti) and res.Expiry if you track revocation.

// Receiver side:
tok, err := verifier.Verify(ctx, res.Token)
if err != nil {
    return err
}
_ = tok // verified claims; default leeway 30s
```

### Scope policy over OIDC claims

Build the registry once, freeze it, and enforce. `oidc.ScopesOf` (the transport adapter's helper) extracts granted scopes from `*Claims`,
so a token's `scope` claim drives the decision.

```go
import (
    "github.com/altessa-s/go-atlas/auth/scope"
    grpcoidc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
)

reg := scope.NewRegistry()
reg.Register("/files.v1.Files/Read", "files:read")
reg.Register("/files.v1.Files/Write", "files:write")
reg.Register("/health.v1.Health/Check", "") // empty scope = public
reg.Freeze()

enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(grpcoidc.ScopesOf, scope.Exact()))

if err := enf.Enforce(claims, "/files.v1.Files/Write"); err != nil {
    // errors.Is(err, scope.ErrAccessDenied)
}
```

Compose extra rules (superuser, tenant) by wrapping the authorizer; the core stays free of those fields. If the service models access as
roles, `scope.RoleAuthorizer(rolesOf, scope.NewRoleScopes(table), scope.Exact())` maps roles to scopes with roles still supplied by the
caller. See [scope.md](scope.md).

### OPA policy with audit

Compile a Rego bundle into a `Manager`, then evaluate request input against it. Attach an `audit.Recorder` so every decision is recorded.

```go
import (
    "github.com/altessa-s/go-atlas/auth/audit"
    "github.com/altessa-s/go-atlas/auth/opa"
)

bundle := opa.NewPolicyBundle(map[string][]byte{
    "authz.rego": []byte(`package authz
default allow = false
allow { input.role == "admin" }`),
})

rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))
mgr, err := opa.NewManager(ctx, bundle, "data.authz.allow", opa.WithAuditRecorder(rec))
if err != nil {
    return err
}

res, err := mgr.Evaluator().Evaluate(ctx, map[string]any{"role": userRole})
if err != nil {
    return err
}
if !res.Allow {
    return errForbidden // res.DecisionID ties the denial to the audit record
}
```

### Full gRPC server: OIDC + scope + audit

The three layers composed on one interceptor: validate the bearer token, enforce the scope policy on the resulting claims, and record both.

```go
import (
    "github.com/altessa-s/go-atlas/auth/audit"
    "github.com/altessa-s/go-atlas/auth/audit/sinks/dataaudit"
    "github.com/altessa-s/go-atlas/auth/scope"
    grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
    grpcoidc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
)

rec := audit.NewRecorder(dataaudit.New(auditor)) // deny-only, best-effort by default

interceptor := grpcauth.ServerInterceptor(
    grpcauth.WithAuthFn(grpcoidc.AuthFunc(validator,                    // AuthN → *Claims
        grpcoidc.WithAudit(rec, func(c *grpcoidc.Claims) string { return c.Subject }))),
    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf,               // AuthZ on *Claims
        grpcauth.WithScopeAudit(rec, func(c *grpcoidc.Claims) string { return c.Subject }))),
    grpcauth.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
)

// Register on the framework's gRPC server builder:
builder.WithInterceptor(interceptor) // *grpc/server/factory.ServerBuilder
```

Here `validator` is any `grpcoidc.Validator` (`ValidateToken(ctx, token) (*Claims, error)`) and `enf` is a
`*scope.Enforcer[*grpcoidc.Claims]` built as in the recipe above.

## Cross-Cutting Concerns

- **Audit.** Any AuthN/AuthZ decision can be recorded through an `audit.Recorder` without coupling the deciding engine to storage. The adapter
  hooks (`WithScopeAudit`, `static`/`oidc` `WithAudit`, `opa.WithAuditRecorder`) are non-breaking opt-ins. See [audit.md](audit.md).
- **Metrics.** `oidc`, `opa`, `selfjwt`, and `static` expose Prometheus telemetry under stable subsystems (`auth_oidc`, `auth_opa`, …); see
  [../metrics.md](../metrics.md). Metrics aggregate; audit records each decision. They answer different questions, so keep both.
- **Defaults & clock skew.** Token validators share a consistent default leeway of `30s` (`oidc`, `selfjwt`); error sentinels are aligned
  (`ErrTokenInvalid`, `ErrTokenEmpty`, `ErrRateLimited`) so adapters map them uniformly to gRPC codes / HTTP status.
- **Hot-reload.** `oidc` rotates JWKS keys and `opa` reloads policy from its source without a restart, keeping decisions current.

## Design Principles

- **Standards-based** — OIDC discovery (RFC 8414), token introspection (RFC 7662), OPA Rego policies.
- **Secure by default** — asymmetric-only signing, deny-by-default authorization, revocation checks, checksum-verified policy files; an
  unconfigured dependency falls back to a no-op rather than silently allowing access.
- **AuthN/AuthZ separation** — authenticators yield a principal; authorizers consume one. The core authorization packages own no identity type
  and bake in no superuser/tenant/role fields — those live in the caller's `Authorizer`.
- **Consumer-side seams** — extension points (`KeyResolver`, `Authorizer`, `Sink`, gRPC `Auth`/`ClientAuth`) are narrow interfaces defined for
  the consumer; transports adapt the same core.
- **Factory-driven config** — `oidc` and `opa` build from `config.*` structs loaded by `config/loader`; everything else configures through
  generated functional options.

## See Also

- [oidc.md](oidc.md) · [opa.md](opa.md) · [scope.md](scope.md) · [selfjwt.md](selfjwt.md) · [static.md](static.md) · [audit.md](audit.md)
- [`auth/` package README](../../auth/README.md) — code-level quick reference and package listing.
- [../architecture.md](../architecture.md) — overall package map and layering.
- [../configuration.md](../configuration.md) — multi-source config loading for the factory path.
- [../metrics.md](../metrics.md) — auth subsystem metrics reference.
