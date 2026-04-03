# OpenID Connect (OIDC) Authentication

JWT token validation with automatic JWKS rotation, claims validation, presets, introspection, and revocation.

---

## Overview

The `auth/oidc` package provides a transport-agnostic OIDC Provider that handles the full token validation lifecycle:

1. **Discovery** — fetches OIDC metadata from the well-known endpoint.
2. **JWKS** — loads and caches JSON Web Key Sets with automatic or scheduled refresh.
3. **Validation** — verifies signatures, standard claims, custom claims, and CEL rules.
4. **Caching** — optional token cache avoids redundant validation.
5. **Revocation** — checks tokens against a revocation store (local, remote, or bloom filter).
6. **Introspection** — RFC 7662 token introspection for opaque tokens.

The provider integrates with the gRPC auth interceptor, HTTP middleware, health checks, and Prometheus metrics.

## Package Map

```
auth/oidc/                         Core provider, validation, presets, CEL, revocation
auth/oidc/factory/                 Config-based ProviderBuilder (from config.OIDC)
transport/grpc/interceptors/auth/oidc/           gRPC AuthFunc adapter + Claims struct
transport/grpc/interceptors/auth/oidc/validator/ DefaultValidator for gRPC
config/auth_oidc.go                              YAML configuration structures
```

## Quick Start

### Standalone Provider

```go
provider, err := oidc.NewProvider(ctx,
    "https://auth.example.com/.well-known/openid-configuration",
    oidc.WithDefaultValidationOptions(
        oidc.WithValidationIssuer("https://auth.example.com"),
        oidc.WithValidationAudience("my-service"),
    ),
    oidc.WithLogger(logger),
)
if err != nil {
    return err
}
defer provider.Close()

claims, err := provider.ValidateToken(ctx, bearerToken)
// claims is map[string]any with all JWT claims
```

### With Config + Factory

```go
provider, err := oidcfactory.New(cfg.Auth.OIDC).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseTokenCache(redisCache).
    Build(ctx)
```

### gRPC Interceptor Integration

```go
// Create the validator
v := validator.NewDefaultValidator(provider)

// Wire into the auth interceptor
authInterceptor := auth.ServerInterceptor(
    auth.WithAuthFn(oidcgrpc.AuthFunc(v)),
    auth.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
)
```

After authentication, retrieve typed claims from context:

```go
func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
    claims, ok := oidcgrpc.ClaimsFromContext(ctx)
    if !ok {
        return nil, status.Error(codes.Unauthenticated, "no claims")
    }
    userID := claims.Subject
    email  := claims.Email
    scopes := claims.Scopes // sorted []string
    // ...
}
```

## Validation Presets

Presets are named, reusable sets of validation rules. They can be selected manually or automatically via matcher rules.

### Defining Presets

```go
provider, _ := oidc.NewProvider(ctx, discoveryURL,
    oidc.WithPresets(
        oidc.NewValidationPreset("admin",
            oidc.WithValidationRequiredScopes("admin:read", "admin:write"),
            oidc.WithValidationRequireAuthorizedParty(),
        ),
        oidc.NewValidationPreset("service-account",
            oidc.WithValidationAllowMissingSubject(),
            oidc.WithValidationAllowedClientIDs("backend-service"),
        ),
    ),
)

// Use explicitly:
claims, err := provider.ValidateTokenWithPreset(ctx, token, "admin")
```

### Automatic Preset Selection

```go
oidc.WithPresetRules(
    oidc.PresetRule{
        Priority:   1,
        Matcher:    oidc.HasScope("admin:read"),
        PresetName: "admin",
    },
    oidc.PresetRule{
        Priority:   2,
        Matcher:    oidc.ClientIDEquals("backend-service"),
        PresetName: "service-account",
    },
)
```

When `ValidateToken` is called without an explicit preset, the provider evaluates rules by priority and applies the first matching preset.

## Matchers

Matchers test token claims for preset selection. They can be composed with boolean combinators.

| Matcher | Description |
|---------|-------------|
| `ClaimEquals(claim, value)` | String claim equals value |
| `ClaimContains(claim, sub)` | String claim contains substring |
| `ClaimExists(claim)` | Claim is present |
| `HasScope(scope)` | Token has scope |
| `HasAnyScope(scopes...)` | At least one scope present |
| `HasAllScopes(scopes...)` | All scopes present |
| `ClientIDEquals(id)` | `client_id` or `azp` equals value |
| `IssuerEquals(issuer)` | `iss` claim matches |
| `AudienceContains(aud)` | `aud` contains value |
| `CELMatcher(ctx, expr)` | CEL expression against claims |
| `MatcherAnd(m...)` | All must match |
| `MatcherOr(m...)` | Any must match |
| `MatcherNot(m)` | Invert result |

## CEL Validation

Custom CEL expressions are evaluated against a `claims` variable (map[string]any):

```go
oidc.WithValidationCelRules(
    oidc.CELValidationRule{
        Name:       "require-org",
        Expression: `has(claims.org_id) && claims.org_id != ""`,
        Message:    "organization claim required",
    },
    oidc.CELValidationRule{
        Name:       "email-domain",
        Expression: `claims.email.endsWith("@example.com")`,
    },
)
```

CEL expressions are compiled once and cached (LRU, max 1000 entries). Evaluation is limited to 10,000 cost units per rule.

## Token Revocation

Three revocation item types are supported: `"token"` (full JWT), `"jti"` (claim), `"kid"` (key ID).

### With Probabilistic Filter

```go
oidc.WithRevocationStorage(
    oidc.NewFilterRevocationStorage(
        probfilter.NewBloomFilter(1_000_000, 0.001),
        oidc.URLRevocationLoader("https://auth.example.com/revoked-tokens"),
    ),
),
oidc.WithRevocationSyncSchedule("*/30 * * * * *"), // sync every 30s
```

### With Custom Storage

```go
oidc.WithRevocationStorage(myRedisRevocationStorage)
```

## Token Introspection (RFC 7662)

For opaque tokens or additional token metadata:

```go
oidc.WithIntrospection("client-id", "client-secret"),
```

The provider calls the introspection endpoint and returns an `IntrospectionResponse` with active status, scopes, and claims.

## Health & Metrics

### Health Check

`Provider` implements `health.Checker`:

```go
coordinator.RegisterService("oidc", provider)
```

Returns `StatusServing` when the discovery document is valid and JWKS keys are loaded.

### Prometheus Metrics

| Metric | Description |
|--------|-------------|
| `oidc_token_validations_total` | Total validation attempts |
| `oidc_validation_errors_total` | Validation failures |
| `oidc_validation_duration_seconds` | Validation latency histogram |
| `oidc_cache_hits_total` | Token cache hits |
| `oidc_cache_misses_total` | Token cache misses |
| `oidc_jwks_refreshes_total` | JWKS refresh count |
| `oidc_jwks_refresh_errors_total` | JWKS refresh failures |
| `oidc_revocation_check_errors_total` | Revocation check failures |

All metrics are labeled with `issuer`.

## Scheduled Background Tasks

When a scheduler is provided, the provider registers automatic tasks:

```go
oidc.WithScheduler(scheduler),
oidc.WithJWKSRefreshSchedule("0 */5 * * * *"),       // every 5 min
oidc.WithRevocationSyncSchedule("*/30 * * * * *"),    // every 30s
```

Without a scheduler, call `provider.RefreshJWKS(ctx)` manually or rely on automatic refresh on cache miss.

## Configuration (YAML)

```yaml
auth:
  oidc:
    discovery_url: "https://auth.example.com/.well-known/openid-configuration"
    clock_skew: 10s
    cache:
      enabled: true
      tokens_key_prefix: "tokens:"
      revoked_tokens_key_prefix: "revoked-tokens:"
    validation:
      issuer: "https://auth.example.com"
      max_token_lifetime: 24h
      leeway: 10s
      claims:
        required: ["sub", "aud", "exp", "iat", "iss"]
    jwks:
      refresh_enabled: true
      refresh_schedule: "0 */5 * * * *"
      http_timeout: 30s
    client_credentials:
      client_id: "my-service"
      client_secret: "${OIDC_CLIENT_SECRET}"
    introspection:
      enabled: true
    presets:
      list:
        - name: admin
          scopes: ["admin:read", "admin:write"]
          require_authorized_party: true
      selectors:
        - priority: 1
          matcher: "has_scope"
          value: "admin:read"
          preset: admin
    revocation:
      enabled: true
      item_type: "jti"
      sync_enabled: true
      sync_schedule: "*/30 * * * * *"
```

## gRPC Claims Struct

The `transport/grpc/interceptors/auth/oidc` package provides typed access to common claims:

```go
type Claims struct {
    Subject            string
    PreferredUsername   string
    Email              string
    Issuer             string
    Audience           []string
    Scopes             []string   // sorted
    ExpiresAt          time.Time
    IssuedAt           time.Time
    NotBefore          time.Time
    FamilyName         string
    Name               string
    GivenName          string
    RawClaims          map[string]any
}
```

Retrieve from context via `oidcgrpc.ClaimsFromContext(ctx)`.
