# oidc

```go
import "github.com/altessa-s/go-atlas/auth/oidc"
```

Package `oidc` provides OpenID Connect JWT token validation with automatic JWKS key rotation. Supports signature verification, claims validation, token
introspection (RFC 7662), validation presets with matchers (native and CEL-based), and userinfo retrieval.

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
| `Principal(claims, …)` | Builds a canonical `principal.Principal` from validated claims (no extra verification) |

## Provider options

| Option                          | Default            | Description                                            |
|---------------------------------|--------------------|--------------------------------------------------------|
| `WithHTTPClient`                | required | Shared `*http.Client` for every outbound OIDC call; the factory injects the resilient atlas client |
| `WithJwksHTTPTimeout`           | 30s                | Timeout for JWKS HTTP requests                         |
| `WithTokenCache`                | nil                | Cacher implementation for validated token caching      |
| `WithTokensCacheKeyPrefix`      | `tokens:`          | Cache key prefix for validated tokens                  |
| `WithRevokedTokensCacheKeyPrefix` | `revoked-tokens:` | Cache key prefix for introspection-inactive (revoked) results |
| `WithActiveTokensCacheKeyPrefix` | `active-tokens:`  | Cache key prefix for introspection-active results      |
| `WithDefaultValidationOptions`  | --                 | Default validation options applied to all tokens       |
| `WithPresets`                   | --                 | Named validation presets for reusable rule sets        |
| `WithPresetRules`               | --                 | Rules for automatic preset selection by claims         |
| `WithIntrospection`             | disabled           | Enable RFC 7662 introspection with client credentials  |
| `WithIntrospectionFailOpen`     | fail-closed        | Accept a signature-valid token when introspection errors |
| `WithRevocationStorage`         | nil                | Storage backend for token revocation checks            |
| `WithRevocationFilter`          | nil                | Probabilistic filter wrapped in `NewFilterRevocationStorage` when no storage is set |
| `WithRevocationLoader`          | nil                | `DataLoader` that (re)populates the revocation filter on sync |
| `WithRevocationItemType`        | `token`            | What revocation entries identify (`token`, `jti`, `kid`) |
| `WithRevocationAuthoritative`   | nil                | Exact store confirming filter hits (see Revocation accuracy) |
| `WithRevocationFailOpen`        | fail-closed        | Accept tokens on storage errors and start on a failed initial sync |
| `WithRevocationInitialSyncWait` | 2m                 | How long the initial sync retries while a shared filter is busy |
| `WithJWKSRefreshSchedule`       | --                 | Local cron schedule for JWKS refresh                   |
| `WithJWKSMaxStaleness`          | 0 (off)            | Max age of the cached JWKS before `WithJWKSFailureMode` applies |
| `WithJWKSFailureMode`           | enforce            | Stale-JWKS handling: `enforce` (`ErrJWKSStale`), `warn`, `disabled` |
| `WithAudienceFailureMode`       | enforce            | Validation without an expected audience: `enforce` (`ErrAudienceNotConfigured`), `warn`, `disabled` |
| `WithRevocationSyncSchedule`    | --                 | Local cron schedule for revocation sync                |
| `WithServiceConfigPath`         | --                 | Path to JSON service configuration file                |
| `WithDiscoveryValidationMode`   | enforce            | Discovery issuer/endpoint checks: `enforce`, `warn`, `disabled` |
| `WithDiscoveryAllowLoopbackHTTP`| off                | Accept plain-http discovery on a loopback issuer host (local IdP); endpoints stay pinned to that host |
| `WithLogger`                    | discard            | Structured logger (`*slog.Logger`)                     |
| `WithCollector`                 | nil                | Metrics collector (subsystem `auth_oidc`)              |
| `WithHealthCoordinator`         | nil                | Register the provider with a health coordinator        |

## Discovered endpoints

| Method                          | Discovery field          |
|---------------------------------|--------------------------|
| `AuthorizationEndpoint()`       | `authorization_endpoint` |
| `TokenEndpoint()`               | `token_endpoint`         |
| `UserinfoEndpoint()`            | `userinfo_endpoint`      |
| `EndSessionEndpoint()`          | `end_session_endpoint`   |
| `RevocationEndpoint()`          | `revocation_endpoint`    |
| `IntrospectionEndpoint()`       | `introspection_endpoint` |

Each returns `""` when the IdP does not advertise the endpoint. In the default `enforce` discovery validation mode every advertised endpoint is HTTPS
on the issuer's host, or plain http on a loopback issuer host with `WithDiscoveryAllowLoopbackHTTP`; `warn` only logs violations and `disabled` skips
the check. The factory's SSRF-protected HTTP client may still refuse loopback addresses; a local IdP needs a client that allows them.

## Revocation accuracy

Filter-backed revocation storage (`NewFilterRevocationStorage`) is built on a probabilistic filter from [`data/probfilter`](../../data/probfilter/).
Such a filter has no false negatives but does have false positives — the Bloom default is 1% (`probfilterconfig.BloomDefaults.FalsePositiveRate`). It
can therefore prove an item is **not** revoked, never that it **is**.

Pass an `Authoritative` store via `WithRevocationAuthoritative` to confirm every filter hit against the exact answer. A false positive then costs one
extra lookup and nothing else:

	storage := oidc.NewFilterRevocationStorage(filter, loader, exactStore)

Without a confirmer the storage runs in lossy mode: an unconfirmed filter hit is reported as revoked. The security invariant still holds — a revoked
item is never allowed — but roughly `falsePositiveRate` of valid tokens are rejected with `ErrTokenRevoked`. Every lookup (full token, `jti` or `kid`)
runs after signature verification, before introspection, and goes through this path.

The confirmer MUST hold the same revocation set the filter is built from. Confirming against an unrelated store turns every hit into "not revoked" and
silently disables revocation — which is why it is an explicit dependency rather than something derived from config.

`NewProvider` syncs the storage once before returning. Storage errors, and a failed initial sync, are fail-closed (`ErrRevocationCheck`) unless
`WithRevocationFailOpen` is set.

Replicas sharing a Redis filter serialize their syncs with a rebuild lease, and each replica completes one sync of its own at startup. When N replicas
start at once the last one waits about `(N − 1) × rebuild duration`; keep `WithRevocationInitialSyncWait` above that (with headroom) or roll out with
limited surge, otherwise the fail mode applies (fail-closed: `NewProvider` fails and the replica restarts).

## Outbound HTTP

Every outbound OIDC call (discovery, JWKS refresh, introspection, userinfo, URL-based revocation loaders) goes through one `*http.Client`. The package
does not build transport policy itself: pass a client with `WithHTTPClient`, typically the resilient client from
[`transport/http/client`](../../transport/http/client/):

```go
provider, err := oidc.NewProvider(ctx, discoveryURL,
    oidc.WithHTTPClient(httpclient.New(
        httpclient.WithRetryMax(3),
    )),
)
```

The client is required: `NewProvider` returns `ErrHTTPClientRequired` without it, so the transport policy (retry, breaker, proxy, SSRF protection) is
always chosen explicitly. [`factory`](factory/) injects the resilient client built from configuration. Sub-components that need the same client (e.g.
URL-based revocation loaders) opt in by implementing `SetHTTPClient(*http.Client)` — the Provider injects its own client at construction.

For YAML-driven proxy configuration via `oidc.proxy`, see the [Proxy guide](../../docs/proxy.md).

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
| `WithValidationRequiredScopes`         | At least one of the listed scopes must be present       |
| `WithValidationMaxTokenLifetime`       | Maximum allowed duration between `iat` and `exp`        |
| `WithValidationCelRules`               | CEL expressions evaluated against claims                |
| `WithValidationValidMethods`           | Allowed JWT signing algorithms (default: asymmetric)    |
| `WithValidationSubject`                | Require `sub` to equal a value                          |
| `WithValidationExpirationRequired`     | Require the `exp` claim                                 |
| `WithValidationNotBeforeRequired`      | Require the `nbf` claim                                 |
| `WithValidationIgnoredClaims`          | Skip these claims in required/expected-claim checks     |
| `WithValidationAllowMissingSubject`    | Accept a token without `sub` despite the default required claims |
| `WithValidationRequireAuthorizedParty` | Require a string `azp` claim                            |
| `WithValidationAllowedAuthorizedParties` | Allowed `azp` values (with `WithValidationRequireAuthorizedParty`) |
| `WithoutValidationClaimsValidation`    | Skip the claim-policy checks (signature still verified) |

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
| `MustCELMatcher`     | `CELMatcher` that panics on an invalid expression          |
| `MatcherAnd`         | All matchers must return true                              |
| `MatcherOr`          | Any matcher must return true                               |
| `MatcherNot`         | Inverts the result of another matcher                      |

## Subpackages

| Package                  | Description                              |
|--------------------------|------------------------------------------|
| [factory](./factory)     | Config-based provider creation           |

## Redirect and JWKS limits

Provider HTTP clients are shallow-cloned, preserving the transport, cookie jar, timeout and caller redirect policy. Redirects must remain on the
original HTTPS origin (scheme, hostname and effective port), in every discovery validation mode. The caller policy can restrict them further.
`WithDiscoveryAllowLoopbackHTTP()` also permits same-origin HTTP redirects on loopback; it does not allow HTTPS downgrades. Violations return
`ErrRedirectNotAllowed`. This applies to discovery, JWKS, userinfo, introspection and the built-in URL revocation loader, including its explicitly
supplied client. Built-in URL loaders and their filter-storage wrappers are cloned for each provider; caller-owned loader wiring stays unchanged. Custom
loaders remain responsible for their own HTTP behavior.

JWKS responses are limited to 1 MiB and 100 raw keys at startup and refresh. Exceeding either limit returns `ErrJWKSTooLarge`. A failed refresh retains
the previous keys and does not advance their freshness timestamp.
