# oidc

```go
import "github.com/altessa-s/go-atlas/auth/oidc"
```

Package `oidc` provides OpenID Connect JWT token validation with automatic JWKS key rotation. Supports signature verification, claims validation,
token
introspection (RFC 7662), validation presets with matchers (native and CEL-based), and userinfo retrieval.

## Key types

| Type / Interface       | Description                                                                                                    |
|------------------------|----------------------------------------------------------------------------------------------------------------|
| `Provider`             | Core OIDC provider: discovery, JWKS, validation, introspection, userinfo                                       |
| `Cacher`               | Cache of signature verifications; policy and revocation re-run on hits                                         |
| `RevocationStorage`    | Interface for checking and managing revoked tokens, JTIs, or KIDs                                              |
| `Authoritative`        | Exact revocation store consulted to confirm probabilistic filter hits                                          |
| `ValidationPreset`     | Named, reusable set of validation options with pre-compiled CEL rules                                          |
| `PresetRule`           | Rule for automatic preset selection based on token claims                                                      |
| `PresetMatcherFunc`    | Function that tests if token claims match a selection rule                                                     |
| `UserInfo`             | User claims retrieved from the OIDC userinfo endpoint                                                          |
| `CELValidationRule`    | Named CEL expression evaluated against token claims                                                            |
| `Principal(claims, …)` | Builds a canonical `principal.Principal` from validated claims (no extra verification)                         |
| `Claims`               | Typed standard OIDC claims (sub, email, iss, aud, scope, exp, …) plus `RawClaims`                              |
| `Claims.Principal`     | Maps `*Claims` to `principal.Principal` (default `tenant`/`roles` claims); used by the transport auth adapters |
| `Validator`            | Verifies a token through a `TokenVerifier` and returns `*Claims`; plain errors                                 |
| `TokenVerifier`        | `ValidateToken(ctx, token) (map[string]any, error)`; satisfied by `*Provider`                                  |
| `ClaimsValidator`      | `ValidateToken(ctx, token) (*Claims, error)`; the port the HTTP and gRPC adapters take                         |
| `ScopesOf`             | Granted scopes of a `*Claims`, for `auth/scope.ScopeAuthorizer`                                                |

## Provider options

| Option                            | Default           | Description                                                                                                       |
|-----------------------------------|-------------------|-------------------------------------------------------------------------------------------------------------------|
| `WithHTTPClient`                  | required          | Shared `*http.Client` for every outbound OIDC call; the factory injects the resilient atlas client                |
| `WithOwnedHTTPClient`             | off               | The provider owns the client: `Close` closes its idle connections (the factory sets it)                           |
| `WithJwksHTTPTimeout`             | 30s               | Timeout for JWKS HTTP requests                                                                                    |
| `WithTokenCache`                  | nil               | Cacher implementation for validated token caching; a trust boundary (see [Token cache trust](#token-cache-trust)) |
| `WithTokensCacheKeyPrefix`        | `tokens:`         | Cache key prefix for validated tokens                                                                             |
| `WithRevokedTokensCacheKeyPrefix` | `revoked-tokens:` | Cache key prefix for introspection-inactive (revoked) results                                                     |
| `WithActiveTokensCacheKeyPrefix`  | `active-tokens:`  | Cache key prefix for introspection-active results                                                                 |
| `WithDefaultValidationOptions`    | --                | Default validation options applied to all tokens                                                                  |
| `WithPresets`                     | --                | Named validation presets for reusable rule sets                                                                   |
| `WithPresetRules`                 | --                | Rules for automatic preset selection by claims                                                                    |
| `WithIntrospection`               | disabled          | Enable RFC 7662 introspection with client credentials                                                             |
| `WithIntrospectionFailOpen`       | fail-closed       | Accept a signature-valid token when introspection errors                                                          |
| `WithRevocationStorage`           | nil               | Storage backend for token revocation checks                                                                       |
| `WithRevocationFilter`            | nil               | Probabilistic filter wrapped in `NewFilterRevocationStorage` when no storage is set                               |
| `WithRevocationFilterCleanup`     | nil               | Release a dedicated filter on construction failure or provider close                                              |
| `WithRevocationLoader`            | nil               | `probfilter.DataLoader` that (re)populates the revocation filter on sync                                                     |
| `WithRevocationItemType`          | `token`           | What revocation entries identify (`token`, `jti`, `kid`); a token without a string `jti`/`kid` is rejected (`ErrTokenInvalid`) |
| `WithInsecureRevocationSkipMissingItem` | off         | Look such a token up by the whole token instead of rejecting it (it can never match a `jti`/`kid` entry)           |
| `WithRevocationAuthoritative`     | nil               | Exact store confirming filter hits (see Revocation accuracy)                                                      |
| `WithRevocationFailOpen`          | fail-closed       | Accept tokens on storage errors and start on a failed initial sync                                                |
| `WithRevocationInitialSyncWait`   | 2m                | How long the initial sync retries while a shared filter is busy                                                   |
| `WithRevocationMaxStaleness`      | 3h                | Max age of the last successful scheduled revocation sync before "not revoked" answers are rejected; ≤ 0 disables  |
| `WithRevocationSyncTimeout`       | 5m                | Time limit of one scheduled revocation sync; 0 disables                                                           |
| `WithRevocationAllowEmpty`        | off               | Treat an empty sync result as authoritative and empty the filter instead of failing the sync                      |
| `WithJWKSRefreshSchedule`         | --                | Local cron schedule for JWKS refresh                                                                              |
| `WithJWKSMaxStaleness`            | 24h               | Max age of the cached JWKS before `WithJWKSFailureMode` applies; zero disables                                    |
| `WithJWKSFailureMode`             | enforce           | Stale-JWKS handling: `enforce` (`ErrJWKSStale`), `warn`, `disabled`                                               |
| `WithAudienceFailureMode`         | enforce           | Validation without an expected audience: `enforce` (`ErrAudienceNotConfigured`), `warn`, `disabled`               |
| `WithRevocationSyncSchedule`      | --                | Local cron schedule for revocation sync                                                                           |
| `WithDiscoveryValidationMode`     | enforce           | Discovery issuer/endpoint checks: `enforce`, `warn`, `disabled`                                                   |
| `WithDiscoveryAllowLoopbackHTTP`  | off               | Accept plain-http discovery on a loopback issuer host (local IdP); endpoints stay pinned to that host             |
| `WithLogger`                      | discard           | Structured logger (`*slog.Logger`)                                                                                |
| `WithMeter`                       | nil               | Metrics collector (subsystem `auth_oidc`)                                                                         |
| `WithHealthCoordinator`           | nil               | Register the provider with a `health.Registrar`                                                                   |

For a JSON service policy, pass the options returned by the [OIDC factory](../../factory/auth/oidc/#json-service-policy) to `NewProvider`.

## Discovered endpoints

| Method                    | Discovery field          |
|---------------------------|--------------------------|
| `AuthorizationEndpoint()` | `authorization_endpoint` |
| `TokenEndpoint()`         | `token_endpoint`         |
| `UserinfoEndpoint()`      | `userinfo_endpoint`      |
| `EndSessionEndpoint()`    | `end_session_endpoint`   |
| `RevocationEndpoint()`    | `revocation_endpoint`    |
| `IntrospectionEndpoint()` | `introspection_endpoint` |

Each returns `""` when the IdP does not advertise the endpoint. In the default `enforce` discovery validation mode every advertised endpoint is HTTPS
on the issuer's host, or plain http on a loopback issuer host with `WithDiscoveryAllowLoopbackHTTP`; `warn` only logs violations and `disabled` skips
the check. The factory defaults to blocking loopback addresses. Use `oidc.ssrf` to exempt the address of an HTTPS identity provider on loopback.

Introspection requires an explicit boolean `active` member. Invalid responses follow the configured introspection failure policy and leave both the
introspection cache and revocation storage untouched. Introspection, UserInfo and URL revocation fetch failures report the operation, numeric HTTP
status (zero before a response), and a fixed cause classification. Use `errors.Is` for cause matching.

## Revocation accuracy

Filter-backed revocation storage (`NewFilterRevocationStorage`) is built on a probabilistic filter from [`cache/probfilter`](../../cache/probfilter/).
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

A provider that syncs on `WithRevocationSyncSchedule` also bounds how long it trusts the last successful sync. Past `WithRevocationMaxStaleness`
(`DefaultRevocationMaxStaleness`, 3h: three periods of the default hourly schedule) a "not revoked" answer fails the check with `ErrRevocationCheck`
matching `ErrRevocationStale`, unless `WithRevocationFailOpen` is set. Tokens are rejected once the revocation source has been unreachable for longer
than the bound. Keep the bound at several sync intervals (`NewProvider` logs a warning when the schedule is sparser than the bound), or pass zero or a
negative value to turn it off. Without a scheduled sync the bound does not apply. A fail-open provider that started on a failed initial sync counts the
bound from construction.

Replicas sharing a Redis filter rebuild one at a time; a replica whose tick finds a peer rebuilding (`ErrRebuildInProgress`) skips it without counting
it as a sync. The bound is checked against the newest of the provider's own last successful sync and the last rebuild committed by any replica, which
the filter reports through `probfilter.RebuildCommitAgeReporter` (for Redis: the server time stored atomically with the commit, and its age computed by
Redis from its own clock in the same read). The provider reads it after each scheduled sync, off the validation path, and records the reported age once
per commit. Monotonic time elapsed since the read began consumes the remaining freshness budget, including time spent waiting for the reply. A custom
storage opts in by implementing `RevocationSyncReporter`.

Each scheduled sync is limited to `WithRevocationSyncTimeout` (5m). `FileRevocationLoader` accepts regular files and checks for cancellation during
reads. The built-in loaders trim entries, skip blank lines and fail the whole load on a line longer than `MaxRevocationEntryBytes` (64 KiB);
`URLRevocationLoader` also caps the body at its `MaxResponseBytes` (`DefaultRevocationMaxResponseBytes`, 256 MiB). A failed sync always keeps the
current set.

A source that turns up empty while the revocation set holds entries fails the sync with `ErrRevocationLoadFailed` instead of un-revoking everything.
"Holds entries" means the last committed list was non-empty. Each rebuild records the number of entries it loaded with its commit, and the guard
reads it from the filter (`probfilter.RebuildLoadReporter`; Bloom and Cuckoo implement it) on every empty result, under the rebuild lease — a peer
replica may have published a list to a filter shared through Redis since this replica's last sync. Items added with `MarkRevoked` are not part of the
list, so an empty list combined with introspection keeps syncing without `WithRevocationAllowEmpty`. When the size is unknown (never rebuilt, key
recreated, committed by a release that did not record it) the filter's item count (`probfilter.StatsProvider`) decides instead; anything unreadable
counts as entries. When the list can legitimately go from non-empty to empty, pass
`WithRevocationAllowEmpty`: an empty result is then authoritative, the filter is rebuilt empty and the sync succeeds, refreshing the staleness clock.
That removes the protection — a source answering `200` with an empty body during an IdP outage un-revokes everything — so enable it only for a source
trusted to be empty when nothing is revoked. The option applies to the built-in filter storage; a custom `RevocationStorage` decides this itself.

Through YAML these are `oidc.revocation.syncTimeout`, `maxResponseBytes` and `allowEmpty`, mapped by [`factory/auth/oidc`](../../factory/auth/oidc/).

## Lifecycle

The context passed to `NewProvider` bounds construction only: discovery, the initial JWKS fetch and the initial revocation sync. The refresh cron
(`WithJWKSRefreshSchedule`, `WithRevocationSyncSchedule`) keeps that context's values but outlives its cancellation, so a startup-timeout context is
safe to pass. Every provider must be closed: only `Close` stops the cron (waiting for a running job). `Close` closes the HTTP client's idle
connections only when the client was handed over with `WithOwnedHTTPClient` (the factory does that for the client it builds); a client the caller
keeps, possibly shared with other components, is left alone.

A storage can signal `ErrRevocationReadinessLost` on lookup to start one background recovery under the same timeout as the scheduled sync. Concurrent
requests and cron ticks share the running sync. `Close` cancels and joins recovery, releases the provider's health registration, and runs any cleanup
transferred with `WithRevocationFilterCleanup`.

A JWKS refresh applies the same key policy as the initial load. A verification key must omit `use` or declare `sig`. If it declares `key_ops`, the list
must include `verify`. Signed UserInfo responses use the same monotonic JWKS staleness budget as access tokens. Among eligible keys, a key of an
unsupported type is skipped and any other invalid key fails the refresh with `ErrDiscovery`, keeping the cached keys and their freshness; so does a
document without a verification key while a key set is in place.

Replicas sharing a Redis filter serialize their syncs with a rebuild lease, and each replica completes one sync of its own at startup. When N replicas
start at once the last one waits about `(N − 1) × rebuild duration`; keep `WithRevocationInitialSyncWait` above that (with headroom) or roll out with
limited surge, otherwise the fail mode applies (fail-closed: `NewProvider` fails and the replica restarts).

## Token cache trust

Signature-cache keys use `oidc-sig-v3`. On upgrade, each token is verified again and its signed date literals are stored in the new namespace.
Registered dates retain their `json.Number` spelling on both paths. CEL receives a separate double-valued view. Numeric preset conditions and expected
claims compare exact dates by value. A quoted preset condition requires a string claim, while a quoted numeric expected claim can match a date.

`WithTokenCache` stores the claims of a signature-verified token and, on a hit, uses them without re-verifying the signature (temporal claims and the
validation policy are still re-evaluated, and the entry is bound to the current key fingerprint). The `Cacher` is therefore part of the trust base:
anyone with write access to it (for example a shared Redis) can plant claims for any token string. Use a cache only this service can write, or none.

## Outbound HTTP

Every outbound OIDC call (discovery, JWKS refresh, introspection, userinfo, URL-based revocation loaders) goes through one `*http.Client`. The package
does not build transport policy itself: pass a client with `WithHTTPClient`, typically the resilient client from
[`go-atlas-net/http/client`](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client):

```go
provider, err := oidc.NewProvider(ctx, discoveryURL,
    oidc.WithHTTPClient(httpclient.New(
        httpclient.WithRetryMax(3),
    )),
)
```

The client is required: `NewProvider` returns `ErrHTTPClientRequired` without it, so the transport policy (retry, breaker, proxy, SSRF protection) is
always chosen explicitly. [`factory`](../../factory/auth/oidc/) injects the resilient client built from configuration. Sub-components that need the
same
client (e.g. URL-based revocation loaders) opt in by implementing `SetHTTPClient(*http.Client)` — the Provider injects its own client at construction.

For YAML settings, see the [Proxy guide](../../docs/proxy.md) for `oidc.proxy` and the [OIDC configuration guide](../../docs/auth/oidc.md#ssrf-protection)
for `oidc.ssrf`.

## Validation options

| Option                                   | Description                                                                |
|------------------------------------------|----------------------------------------------------------------------------|
| `WithValidationIssuer`                   | Expected token issuer (falls back to discovery issuer)                     |
| `WithValidationAudience`                 | Expected audience values                                                   |
| `WithValidationLeeway`                   | Clock skew tolerance for time-based claims; at most `jwt.MaxLeeway` (5m)       |
| `WithValidationIssuedAt`                 | Verify the `iat` claim                                                     |
| `WithValidationRequiredClaims`           | Claims required on top of `DefaultRequiredClaims`                          |
| `WithValidationExpectedClaims`           | Claims that must match values, compared by JSON type; arrays by membership |
| `WithValidationAllowedClientIDs`         | Allowed `client_id` or `azp` values                                        |
| `WithValidationRequiredScopes`           | At least one of the listed scopes must be present                          |
| `WithValidationMinTokenLifetime`         | Minimum allowed duration between `iat` and `exp`. Zero disables it.        |
| `WithValidationMaxTokenLifetime`         | Maximum allowed duration between `iat` and `exp`                           |
| `WithValidationCelRules`                 | CEL expressions evaluated against claims                                   |
| `WithValidationValidMethods`             | Allowed JWT signing algorithms (default: asymmetric)                       |
| `WithValidationSubject`                  | Require `sub` to equal a value                                             |
| `WithValidationNotBeforeRequired`        | Require the `nbf` claim                                                    |
| `WithValidationIgnoredClaims`            | Skip these claims in required/expected-claim checks (never `exp`)          |
| `WithValidationAllowMissingSubject`      | Accept a token without `sub` despite the default required claims           |
| `WithValidationRequireAuthorizedParty`   | Require a string `azp` claim                                               |
| `WithValidationAllowedAuthorizedParties` | Allowed `azp` values (with `WithValidationRequireAuthorizedParty`)         |
| `WithoutValidationClaimsValidation`      | Skip the claim-policy checks (signature still verified)                    |

An enabled lifetime bound requires both `exp` and `iat`. The provider compares their exact difference against the inclusive bounds on every policy
evaluation, including signature-cache hits. A negative minimum or a minimum above an enabled maximum returns `rules.ErrInvalidConfig`.

## Matchers

| Matcher            | Description                                       |
|--------------------|---------------------------------------------------|
| `ClaimEquals`      | String claim equals expected value                |
| `ClaimContains`    | String claim contains substring                   |
| `ClaimExists`      | Claim is present in token                         |
| `HasScope`         | Token contains a specific scope                   |
| `HasAnyScope`      | Token contains at least one of the listed scopes  |
| `HasAllScopes`     | Token contains all listed scopes                  |
| `ClientIDEquals`   | `client_id` (or `azp` fallback) equals value      |
| `IssuerEquals`     | `iss` claim equals expected value                 |
| `AudienceContains` | `aud` claim contains expected value               |
| `CELMatcher`       | CEL expression evaluated against claims           |
| `MustCELMatcher`   | `CELMatcher` that panics on an invalid expression |
| `MatcherAnd`       | All matchers must return true                     |
| `MatcherOr`        | Any matcher must return true                      |
| `MatcherNot`       | Inverts the result of another matcher             |

## Typed claims

`NewValidator(verifier, opts...)` wraps a `TokenVerifier` (typically the `*Provider`) and projects the verified claim set onto `*Claims`, reading
every field through the shared [`auth/jwt`](../jwt) accessors: `aud` accepts a string or an array, `scope` a space-separated string or an array (token
order kept), and `exp`/`iat`/`nbf` decode to `time.Time`. The raw map stays in `Claims.RawClaims`. Errors are returned wrapped, never as a transport
status, so `errors.Is(err, oidc.ErrTokenRevoked)` keeps working; the transport adapters map them —
[`transport/http/server/middlewares/auth/oidc`](../../transport/http/server/middlewares/auth/oidc) to 401 and
[`transport/grpc/interceptors/auth/oidc`](../../transport/grpc/interceptors/auth/oidc) to `codes.Unauthenticated`.

| Option                | Default | Description                                       |
|-----------------------|---------|---------------------------------------------------|
| `WithValidatorLogger` | discard | Logger receiving a record per failed verification |

## Logging of failed validations

A rejected token is an everyday event that any caller can provoke, so `Provider` and `Validator` log it at **Debug**: errors matching
`ErrTokenInvalid`, `ErrCELValidation` or `ErrTokenRevoked` (malformed, expired, badly signed, a failed claim or CEL rule, revoked). Failures of the
provider's own infrastructure keep their **Error**/**Warn** records: a stale JWKS (`ErrJWKSStale`), an unreachable revocation storage
(`ErrRevocationCheck`) or introspection endpoint (`ErrIntrospection`), and anything a `Validator` cannot classify. Count rejections with the
validation error metric rather than the log.

A failed CEL rule is reported by name (`rule 'admin-only' failed`, or the rule's custom `Message`); the expression text is never part of the error.

```go
v := oidc.NewValidator(provider)

claims, err := v.ValidateToken(ctx, token)
if err != nil {
    return err
}
enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(oidc.ScopesOf, scope.Exact()))
```

## Subpackages

| Package                            | Description                    |
|------------------------------------|--------------------------------|
| [factory](../../factory/auth/oidc) | Config-based provider creation |

## Redirect and JWKS limits

Provider HTTP clients are shallow-cloned, preserving the transport, cookie jar, timeout and caller redirect policy. Redirects must remain on the
original HTTPS origin (scheme, hostname and effective port), in every discovery validation mode. The caller policy can restrict them further.
`WithDiscoveryAllowLoopbackHTTP()` also permits same-origin HTTP redirects on loopback; it does not allow HTTPS downgrades. Violations return
`ErrRedirectNotAllowed`. This applies to discovery, JWKS, userinfo, introspection and the built-in URL revocation loader, including its explicitly
supplied client. Built-in URL loaders and their filter-storage wrappers are cloned for each provider; caller-owned loader wiring stays unchanged.
Custom
loaders remain responsible for their own HTTP behavior.

JWKS responses are limited to 1 MiB and 100 raw keys at startup and refresh. Exceeding either limit returns `ErrJWKSTooLarge`. A failed refresh
retains
the previous keys and does not advance their freshness timestamp.

Both named-key and no-`kid` verification use the keys admitted at the last successful JWKS load.
