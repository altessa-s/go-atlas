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
| `Cacher`               | Cache of signature verifications; policy and revocation re-run on hits    |
| `RevocationStorage`    | Interface for checking and managing revoked tokens, JTIs, or KIDs         |
| `Authoritative`        | Exact revocation store consulted to confirm probabilistic filter hits     |
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
| `WithRevocationAuthoritative`   | nil                | Exact store confirming filter hits (see Revocation accuracy) |
| `WithRevocationFailOpen`        | fail-closed        | Accept tokens on storage errors and start on a failed initial sync |
| `WithRevocationInitialSyncWait` | 2m                 | How long the initial sync retries while a shared filter is busy |
| `WithJWKSRefreshSchedule`       | --                 | Local cron schedule for JWKS refresh                   |
| `WithRevocationSyncSchedule`    | --                 | Local cron schedule for revocation sync                |
| `WithServiceConfigPath`         | --                 | Path to JSON service configuration file                |
| `WithLogger`                    | discard            | Structured logger (`*slog.Logger`)                     |

## Revocation accuracy

Filter-backed revocation storage (`NewFilterRevocationStorage`) is built on a probabilistic filter from
[`data/probfilter`](../../data/probfilter/). Such a filter has no false negatives but does have false positives — the Bloom default is
1% (`config.ProbabilisticFilterBloomDefaults.FalsePositiveRate`). It can therefore prove an item is **not** revoked, never that it **is**.

Pass an `Authoritative` store via `WithRevocationAuthoritative` to confirm every filter hit against the exact answer. A false positive
then costs one extra lookup and nothing else:

	storage := oidc.NewFilterRevocationStorage(filter, loader, exactStore)

Without a confirmer the storage runs in lossy mode: an unconfirmed filter hit is reported as revoked. The security invariant still
holds — a revoked item is never allowed — but roughly `falsePositiveRate` of valid tokens are rejected with `ErrTokenRevoked`. Every
lookup (full token, `jti` or `kid`) runs after signature verification, before introspection, and goes through this path.

The confirmer MUST hold the same revocation set the filter is built from. Confirming against an unrelated store turns every hit into
"not revoked" and silently disables revocation — which is why it is an explicit dependency rather than something derived from config.

`NewProvider` syncs the storage once before returning. Storage errors, and a failed initial sync, are fail-closed (`ErrRevocationCheck`)
unless `WithRevocationFailOpen` is set.

Replicas sharing a Redis filter serialize their syncs with a rebuild lease, and each replica completes one sync of its own at startup. When N
replicas start at once the last one waits about `(N − 1) × rebuild duration`; keep `WithRevocationInitialSyncWait` above that (with headroom) or
roll out with limited surge, otherwise the fail mode applies (fail-closed: `NewProvider` fails and the replica restarts).

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
