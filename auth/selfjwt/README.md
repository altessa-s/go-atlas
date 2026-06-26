# selfjwt

```go
import "github.com/altessa-s/go-atlas/auth/selfjwt"
```

Package `selfjwt` mints and verifies self-issued JWTs for services that are both the issuer and the verifier of their own per-subject
tokens. A token is signed with a subject's private key, carries that key's id in the `kid` header, and is later verified by resolving the
subject's public key for that `kid` — so rotating a subject's signing key (issuing a new `kid`) invalidates every token still carrying the
old one.

Unlike [`auth/oidc`](../oidc), which validates tokens from an external identity provider against a remote JWKS, `selfjwt` owns both ends.

## Key types

| Type / Interface  | Description                                                                                  |
|-------------------|----------------------------------------------------------------------------------------------|
| `Minter`          | Issues a signed token for a subject, stamping the signing key's id into the `kid` header      |
| `Verifier`        | Validates a token fail-closed and returns the verified `Token`                                |
| `KeyProvider`     | The single seam to a caller's key storage: resolves signing and verification keys             |
| `SigningKey`      | A subject's current signing material: private key, algorithm, and non-empty key id            |
| `VerificationKey` | A subject's public key for a `kid`, bound to the algorithm it verifies                         |
| `Token`           | The verified result: subject, jti, scopes, expiry                                            |
| `Algorithm`       | A JWA signature algorithm name (e.g. `EdDSA`, `ES256`, `RS256`)                               |
| `Metrics`         | Optional mint/verify telemetry; a nil `*Metrics` is a valid no-op receiver                    |

## Options

| Option                   | Default        | Description                                                                 |
|--------------------------|----------------|-----------------------------------------------------------------------------|
| `WithIssuer`             | `""`           | The `iss` claim set on mint and required on verify                          |
| `WithMaxTokenLifetime`   | 30 days        | Ceiling a requested TTL is clamped to                                       |
| `WithClockSkew`          | 30s            | Leeway applied to `exp` / `nbf` to tolerate clock drift                     |
| `WithCacheTTL`           | 5m             | How long a resolved verification key is cached before reload               |
| `WithCacheMaxEntries`    | 10000          | Hard cap on cached verification keys, bounding memory under subject churn   |
| `WithAllowedAlgorithms`  | EdDSA,ES256,RS256 | Replaces the verifier's asymmetric-only allow-list (HMAC excluded)      |
| `WithMetrics`            | nil (no-op)    | Attach a `*Metrics` for mint/verify counters and latency                    |
| `WithClock`              | system UTC     | Injectable clock (tests)                                                    |
| `WithRand`               | `crypto/rand`  | Injectable randomness source for the jti (tests)                           |

## Security

Verification is fail-closed:

- The algorithm is checked against the allow-list **before** the signature is verified, closing algorithm-confusion attacks. The default
  allow-list is asymmetric only (`EdDSA`, `ES256`, `RS256`); HMAC is intentionally excluded so a symmetric secret can never be confused
  with a public key.
- Each `VerificationKey` must name the `Algorithm` it verifies; the verifier rejects a token whose algorithm is empty or mismatched,
  binding every key to a single scheme.
- An `exp` claim is mandatory, and temporal claims (`exp` / `nbf`) are validated with the configured clock-skew leeway.
- Resolved verification keys are cached per `(subject, kid)` with a TTL and a hard entry cap.

## Usage

```go
provider := myKeyStore{} // implements selfjwt.KeyProvider

minter := selfjwt.NewMinter(provider, selfjwt.WithIssuer("billing"))
res, err := minter.Mint(ctx, selfjwt.MintRequest{
    Subject: tenantID,
    Scopes:  []string{"files:read"},
    TTL:     time.Hour,
})
// res.Token is the serialized JWT; res.ID and res.Expiry are persisted by the caller.

verifier := selfjwt.NewVerifier(provider, selfjwt.WithIssuer("billing"))
tok, err := verifier.Verify(ctx, res.Token)
// tok.Subject, tok.Scopes, tok.Expiry — translate provider sentinels (ErrSubjectUnknown,
// ErrKeyRotated) and ErrTokenExpired / ErrTokenInvalid as the caller sees fit.
```

## Errors

| Sentinel                 | Meaning                                                                       |
|--------------------------|-------------------------------------------------------------------------------|
| `ErrTokenInvalid`        | Malformed token, unexpected algorithm, bad signature, or non-expiry claim failure |
| `ErrTokenExpired`        | `exp` is in the past beyond the clock-skew leeway                             |
| `ErrSubjectUnknown`      | `KeyProvider` has no keys for the subject (propagated unwrapped)              |
| `ErrKeyRotated`          | Subject is known but the `kid` is not (propagated unwrapped)                  |
| `ErrAlgorithmNotAllowed` | Algorithm is unregistered, unmatched, or the resolved key names no algorithm  |
| `ErrSigningKeyInvalid`   | `KeyProvider` returned signing material that cannot be verified (e.g. empty key id) |
