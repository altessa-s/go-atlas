# oidc

```go
import "github.com/altessa-s/go-atlas/auth/oidc"
```

Package `oidc` provides OpenID Connect JWT token validation with automatic JWKS key rotation. Supports signature verification, claims
validation, token introspection (RFC 7662), validation presets with matchers (native and CEL-based), and userinfo retrieval.

## Key types

| Type / Interface       | Description                                                               |
|------------------------|---------------------------------------------------------------------------|
| `Provider`             | Core OIDC provider: discovery, JWKS, validation, introspection, userinfo  |
| `Cacher`               | Token cache interface for avoiding redundant validation                   |
| `RevocationStorage`    | Interface for checking and managing revoked tokens, JTIs, or KIDs         |
| `ValidationPreset`     | Named, reusable set of validation options with pre-compiled CEL rules     |
| `PresetRule`            | Rule for automatic preset selection based on token claims                 |
| `PresetMatcherFunc`    | Function that tests if token claims match a selection rule                |
| `UserInfo`             | User claims retrieved from the OIDC userinfo endpoint                     |
| `CELValidationRule`    | Named CEL expression evaluated against token claims                       |

## Provider options

| Option                          | Default            | Description                                            |
|---------------------------------|--------------------|--------------------------------------------------------|
| `WithHTTPClientOptions`         | resilient defaults | Forward `httpclient.Option` values (proxy, retry, breaker, transport) to the shared OIDC HTTP client |
| `WithJwksHTTPTimeout`           | 30s                | Timeout for JWKS HTTP requests                         |
| `WithTokenCache`                | nil                | Cacher implementation for validated token caching      |
| `WithDefaultValidationOptions`  | --                 | Default validation options applied to all tokens       |
| `WithPresets`                   | --                 | Named validation presets for reusable rule sets        |
| `WithPresetRules`               | --                 | Rules for automatic preset selection by claims         |
| `WithIntrospection`             | disabled           | Enable RFC 7662 introspection with client credentials  |
| `WithRevocationStorage`         | nil                | Storage backend for token revocation checks            |
| `WithScheduler`                 | nil                | Task registrar for background JWKS refresh             |
| `WithJWKSRefreshSchedule`       | --                 | Cron expression for periodic JWKS key rotation         |
| `WithRevocationSyncSchedule`    | --                 | Cron expression for revocation list synchronization    |
| `WithServiceConfigPath`         | --                 | Path to JSON service configuration file                |
| `WithLogger`                    | discard            | Structured logger (`*slog.Logger`)                     |

## Outbound HTTP

Every outbound OIDC call (discovery, JWKS refresh, introspection, userinfo,
URL-based revocation loaders) goes through the same resilient HTTP client
built from [`transport/http/client`](../../transport/http/client/). Configure
proxy, retry, circuit breaker, or custom transport with:

```go
provider, err := oidc.NewProvider(discoveryURL,
    oidc.WithHTTPClientOptions(
        httpclient.WithProxyURL(corpProxy),
        httpclient.WithRetryMax(3),
    ),
)
```

Sub-components that need the same client (e.g. URL-based revocation
loaders) opt in by implementing
[`httpclient.HTTPClientSetter`](../../transport/http/client/injector.go) —
the Provider injects its own client at construction.

For YAML-driven proxy configuration via `oidc.proxy`, see the
[Proxy guide](../../docs/proxy.md).

## Validation options

| Option                                 | Description                                             |
|----------------------------------------|---------------------------------------------------------|
| `WithValidationIssuer`                 | Expected token issuer (falls back to discovery issuer)  |
| `WithValidationAudience`               | Expected audience values                                |
| `WithValidationLeeway`                 | Clock skew tolerance for time-based claims              |
| `WithValidationIssuedAt`               | Verify the `iat` claim                                  |
| `WithValidationRequiredClaims`         | Claims that must be present in the token                |
| `WithValidationExpectedClaims`         | Claims that must match exact values                     |
| `WithValidationAllowedClientIDs`       | Allowed `client_id` or `azp` values                     |
| `WithValidationRequiredScopes`         | At least one scope must be present                      |
| `WithValidationMaxTokenLifetime`       | Maximum allowed duration between `iat` and `exp`        |
| `WithValidationCelRules`               | CEL expressions evaluated against claims                |
| `WithValidationValidMethods`           | Allowed JWT signing algorithms (default: asymmetric)    |

## Matchers

| Matcher              | Description                                                |
|----------------------|------------------------------------------------------------|
| `ClaimEquals`        | String claim equals expected value                         |
| `ClaimContains`      | String claim contains substring                            |
| `ClaimExists`        | Claim is present in token                                  |
| `HasScope`           | Token contains a specific scope                            |
| `HasAnyScope`        | Token contains at least one of the listed scopes           |
| `HasAllScopes`       | Token contains all listed scopes                           |
| `ClientIDEquals`     | `client_id` (or `azp` fallback) equals value               |
| `IssuerEquals`       | `iss` claim equals expected value                          |
| `AudienceContains`   | `aud` claim contains expected value                        |
| `CELMatcher`         | CEL expression evaluated against claims                    |
| `MatcherAnd`         | All matchers must return true                              |
| `MatcherOr`          | Any matcher must return true                               |
| `MatcherNot`         | Inverts the result of another matcher                      |

## Subpackages

| Package                  | Description                              |
|--------------------------|------------------------------------------|
| [factory](./factory)     | Config-based provider creation           |
