# selfjwt

```go
import "github.com/altessa-s/go-atlas/auth/selfjwt"
```

Package `selfjwt` mints and verifies self-issued JWTs for services that are both the issuer and the verifier of their own per-subject tokens. A token
is
signed with a subject's private key, carries that key's id in the `kid` header, and is later verified by resolving the subject's public key for that
`kid` — so rotating a subject's signing key (issuing a new `kid`) lets the verifier reject tokens still carrying the old one. Rejection is immediate
for
a `kid` that was never cached; for an already-cached `kid` it takes effect within at most `WithCacheTTL`, or at once when the caller drops the entry
via
`Verifier.InvalidateKey` / `Verifier.InvalidateSubject` on rotation.

Unlike [`auth/oidc`](../oidc), which validates tokens from an external identity provider against a remote JWKS, `selfjwt` owns both ends.

## Key types

Key and algorithm types come from `github.com/altessa-s/go-atlas/auth/jwt`.

| Type / Interface  | Description                                                                                                                        |
|-------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `Minter`          | Issues a signed token for a subject, stamping the signing key's id into the `kid` header                                           |
| `Verifier`        | Validates a token fail-closed and returns the verified `Token`; `InvalidateKey` / `InvalidateSubject` drop cached keys on rotation |
| `KeyProvider`     | The single seam to a caller's key storage: resolves signing and verification keys                                                  |
| `jwt.SigningKey`      | A subject's current signing material: private key, algorithm, and non-empty key id                                                 |
| `jwt.VerificationKey` | A subject's public key for a `kid`, bound to the algorithm it verifies                                                             |
| `Token`           | The verified result: subject, jti, scopes, expiry; `Principal()` converts it to a `principal.Principal`                            |
| `jwt.Algorithm`       | A JWA signature algorithm name (e.g. `EdDSA`, `ES256`, `RS256`)                                                                    |
| `Metrics`         | Optional mint/verify telemetry; a nil `*Metrics` is a valid no-op receiver                                                         |

## Options

| Option                     | Default           | Description                                                                                               |
|----------------------------|-------------------|-----------------------------------------------------------------------------------------------------------|
| `WithMaxTokenLifetime`     | 30 days           | Ceiling a requested TTL is clamped to                                                                     |
| `WithLeeway`               | 30s               | Leeway applied to `exp` / `nbf` to tolerate clock drift; clamped to `jwt.MaxLeeway` (5m)                      |
| `WithCacheTTL`             | 5m                | How long a resolved verification key is cached before reload                                              |
| `WithCacheMaxEntries`      | 10000             | Hard cap on cached verification keys, bounding memory under subject churn                                 |
| `WithKeyLookupTimeout`     | 10s               | Deadline for a shared key-provider lookup on a cache miss (must be > 0)                                   |
| `WithNegativeCacheTTL`     | 30s               | How long `ErrSubjectUnknown` / `ErrKeyRotated` from the provider is cached; `0` disables                  |
| `WithMaxConcurrentLookups` | 64                | Distinct key-provider lookups in flight; a new one beyond it fails fast with `singleflight.ErrOverloaded` |
| `WithAllowedAlgorithms`    | EdDSA,ES256,RS256 | Replaces the verifier's asymmetric-only allow-list (HMAC excluded)                                        |
| `WithMetrics`              | nil (no-op)       | Attach a `*Metrics` for mint/verify counters and latency                                                  |
| `WithRevocation`           | none              | `jwt.RevocationChecker` consulted with the token's `jti` on verify                                        |
| `WithClock`                | system UTC        | Injectable clock (tests)                                                                                  |
| `WithRand`                 | `crypto/rand`     | Injectable randomness source for the jti (tests)                                                          |

## Security

Verification is fail-closed:

- The algorithm is checked against the allow-list **before** the signature is verified, closing algorithm-confusion attacks. The default allow-list is
  asymmetric only (`EdDSA`, `ES256`, `RS256`); HMAC is intentionally excluded so a symmetric secret can never be confused with a public key.
- Each `jwt.VerificationKey` must set its `Algorithm` field to the algorithm it verifies; the verifier rejects a token whose algorithm is empty or mismatched, binding every key
  to a single scheme.
- An `exp` claim is mandatory, and temporal claims (`exp` / `nbf`) are validated with the configured clock-skew leeway.
- The `issuer` and `audience` arguments of `NewMinter` / `NewVerifier` are required: the minter writes them as `iss` / `aud`, the verifier rejects
  any other value, and an empty one fails `Mint` / `Verify` with `ErrIssuerRequired` / `ErrAudienceRequired`. Services that share a `KeyProvider`
  therefore cannot replay each other's tokens.
- Resolved verification keys are cached per `(subject, kid)` with a TTL and a hard entry cap. Concurrent misses for the same key are collapsed into a
  single `KeyProvider` lookup (singleflight), so a burst of requests for an uncached subject cannot stampede the provider. Each caller waits under its
  own context and returns its cancellation or deadline error; the shared lookup is detached from any one caller and bounded by `WithKeyLookupTimeout`
  instead.
- The token's subject and kid are read before its signature is verified, so they are attacker-controlled. A provider miss (`ErrSubjectUnknown`,
  `ErrKeyRotated`) is cached for `WithNegativeCacheTTL` and distinct lookups in flight are capped by `WithMaxConcurrentLookups`, so a flood of
  tokens with random subjects or kids cannot drive one provider lookup per request. A newly provisioned subject or kid is rejected from the
  negative cache until it expires or `InvalidateSubject` / `InvalidateKey` is called.
- The cache bounds the rotation-to-rejection window: an already-cached `kid` keeps verifying for up to `WithCacheTTL` after rotation, because a cache
  hit skips the `KeyProvider` (so its `ErrKeyRotated` is not seen). To revoke immediately, call `Verifier.InvalidateKey` (or `InvalidateSubject`) on
  rotation, or set a short `WithCacheTTL` (`0` disables caching for strict, no-lag verification). Invalidation also fences lookups in flight: they
  cannot re-cache a retired key, and later verifications do not share them.

## Usage

```go
provider := myKeyStore{} // implements selfjwt.KeyProvider

minter := selfjwt.NewMinter(provider, "billing", "billing-api")
res, err := minter.Mint(ctx, selfjwt.MintRequest{
    Subject: tenantID,
    Scopes:  []string{"files:read"},
    TTL:     time.Hour,
})
// res.Token is the serialized JWT; res.ID and res.Expiry are persisted by the caller.

verifier := selfjwt.NewVerifier(provider, "billing", "billing-api")
tok, err := verifier.Verify(ctx, res.Token)
// tok.Subject, tok.Scopes, tok.Expiry — translate provider sentinels (ErrSubjectUnknown,
// ErrKeyRotated) and ErrTokenExpired / ErrTokenInvalid as the caller sees fit.
```

When one process both issues and verifies its own tokens, build the pair with `New` so one issuer, audience and option list configure both sides and
their shared settings (issuer, audience, clock) cannot drift apart:

```go
minter, verifier := selfjwt.New(provider, "billing", "billing-api")
```

## Errors

| Sentinel                 | Meaning                                                                                                        |
|--------------------------|----------------------------------------------------------------------------------------------------------------|
| `ErrTokenInvalid`        | Malformed token, unexpected algorithm, bad signature, foreign `iss` / `aud`, or other non-expiry claim failure |
| `ErrTokenExpired`        | `exp` is in the past beyond the clock-skew leeway                                                              |
| `ErrSubjectUnknown`      | `KeyProvider` has no keys for the subject (propagated unwrapped)                                               |
| `ErrKeyRotated`          | Subject is known but the `kid` is not (propagated unwrapped)                                                   |
| `ErrAlgorithmNotAllowed` | Algorithm is unregistered, unmatched, or the resolved key names no algorithm                                   |
| `ErrSigningKeyInvalid`   | `KeyProvider` returned signing material that cannot be verified (e.g. empty key id)                            |
| `ErrSubjectRequired`     | `MintRequest.Subject` is empty (`Minter` rejects it before signing)                                            |
| `ErrIssuerRequired`      | The minter or verifier was built with an empty issuer                                                          |
| `ErrAudienceRequired`    | The minter or verifier was built with an empty audience                                                        |
| `jwt.ErrTokenRevoked`    | The token's `jti` is revoked per `WithRevocation` (propagated unwrapped)                                       |
