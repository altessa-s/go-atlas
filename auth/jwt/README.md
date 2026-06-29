# jwt

```go
import "github.com/altessa-s/go-atlas/auth/jwt"
```

Package `jwt` is a generic, low-level toolkit for signing and verifying JWTs on top of
[`github.com/golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt). It is the shared core beneath the repository's specialized auth
packages — [`auth/selfjwt`](../selfjwt) (self-issued per-subject tokens) and [`auth/oidc`](../oidc) (external OIDC providers) — factoring
out the security policy they would otherwise each re-derive: an algorithm allow-list, an algorithm-confusion guard, standard
temporal/issuer/audience claim validation, and claim extraction.

Use it directly to sign and verify tokens without committing to a JWKS or a particular key store: verification resolves keys through a
caller-supplied [`KeyResolver`](#key-resolution), so resolving by `kid` against a JWKS and resolving by a claim such as `sub` plus `kid`
both fit the same seam.

## Key types

| Type / Interface  | Description                                                                                          |
|-------------------|-----------------------------------------------------------------------------------------------------|
| `Signer`          | Signs a `Claims` set with a `SigningKey`, stamping the key id into the `kid` header                  |
| `Verifier`        | Validates a token fail-closed and returns the verified `Claims`                                      |
| `Verifier.VerifyWithHeader` | Like `Verify`, but also returns the verified `Header` (e.g. `kid` for a revocation lookup) |
| `Verifier.VerifySignature` | Verifies signature + algorithm only, skipping claim validation (returns claims + header)    |
| `Verifier.ValidateClaims`  | Validates an already-parsed `Claims` set against the policy; no signature check              |
| `Claims`          | The decoded payload as a map, with typed accessors (`Subject`, `Audience`, `Expiry`, `Scopes`, ...)  |
| `Claims.Decode`   | Unmarshal the whole claim set into a service-defined struct (json tags); `Get[T]` reads one claim    |
| `KeyResolver`     | The single seam to a caller's key storage: maps a token's `Header` and unverified `Claims` to a key  |
| `KeyResolverFunc` | Adapts a function to a `KeyResolver`; `StaticKey` wraps one fixed key                                |
| `SigningKey`      | Signing material: private key, algorithm, and non-empty key id                                       |
| `VerificationKey` | A public key, optionally bound to the one algorithm it may verify                                    |
| `Header`          | The `alg` / `kid` / `typ` a resolver needs to locate a key                                           |
| `Algorithm`       | A JWA signature algorithm name (e.g. `EdDSA`, `ES256`, `RS256`)                                       |

## Options

A single `Option` type configures both `Signer` and `Verifier`.

| Option                  | Default            | Description                                                                |
|-------------------------|--------------------|----------------------------------------------------------------------------|
| `WithIssuer`            | `""`               | The `iss` claim set by `NewClaims` and required on verify                   |
| `WithLeeway`            | 30s                | Clock-skew tolerance applied to `exp` / `nbf` / `iat`                       |
| `WithMaxTokenLifetime`  | 30 days            | Ceiling a `NewClaims` TTL is clamped to                                     |
| `WithExpirationRequired`| enabled            | Make an `exp` claim mandatory on verify (the fail-closed default)          |
| `WithExpirationOptional`| —                  | Reverse the default: accept a token without `exp` (still validated if present) |
| `WithAllowedAlgorithms` | EdDSA,ES256,RS256  | Replaces the asymmetric-only allow-list, for signing and verifying         |
| `WithAudiences`         | unchecked          | Acceptable `aud` values; a token is accepted when its `aud` intersects them |
| `WithRequiredClaims`    | none               | Claims that must be present beyond the temporal ones                        |
| `WithTokenType`         | `JWT`              | typ header written by `Signer.Sign` (e.g. `TypeAccessToken` / `at+jwt`)     |
| `WithExpectedTokenType` | unchecked          | Require the token's typ header on verify (case-/`application/`-insensitive) |
| `WithSubject`           | unchecked          | Require the `sub` claim to equal a value on verify                          |
| `WithIssuedAt`          | disabled           | Validate the `iat` claim, rejecting a future-issued token                   |
| `WithNotBeforeRequired` | disabled           | Require the `nbf` claim to be present on verify                             |
| `WithClock`             | system UTC         | Injectable clock (tests)                                                    |
| `WithRand`              | `crypto/rand`      | Injectable randomness source for the jti (tests)                           |

## Key resolution

`Verifier` never assumes where keys come from. It calls a `KeyResolver` with the token's `Header` and its **still-unverified** `Claims`,
then verifies the signature against the returned `VerificationKey`. A resolver may pin an algorithm to the key via
`VerificationKey.Algorithm`; when set, the verifier rejects a token whose `alg` does not match, so a key cannot be coerced into another
scheme. Leave it empty to let the allow-list alone govern the algorithm (the usual case for JWKS-resolved keys).

```go
resolver := jwt.KeyResolverFunc(func(ctx context.Context, hdr jwt.Header, unverified jwt.Claims) (jwt.VerificationKey, error) {
    pub, err := keys.Lookup(ctx, unverified.Subject(), hdr.Kid) // resolve by subject + kid, or by kid alone
    if err != nil {
        return jwt.VerificationKey{}, err // propagated unwrapped, so callers can map their own sentinels
    }
    return jwt.VerificationKey{Algorithm: jwt.AlgES256, Key: pub}, nil
})
```

## Custom claims

`Claims` is a `map[string]any`, so service-specific fields need no schema — set them when signing and read them
after verifying:

```go
claims, _ := signer.NewClaims(userID, time.Hour)
claims.Set("role", "admin").Set("groups", []string{"eng", "ops"})
raw, _ := signer.Sign(key, claims)

verified, _ := verifier.Verify(ctx, raw)
role, ok := jwt.Get[string](verified, "role")            // typed single-claim read (numbers are float64)
var mine struct {                                        // or bind the whole set to a service struct
    Role   string   `json:"role"`
    Groups []string `json:"groups"`
}
_ = verified.Decode(&mine)
```

Registered claims keep their typed accessors (`Subject`, `Audience`, `Expiry`, `Scopes`, ...); `Get[T]` /
`Decode` cover everything else.

### Typed claims

A service that prefers a struct over a map embeds `ClaimsBase` (the registered claims, with `ExpiryTime` / `NotBeforeTime` /
`IssuedAtTime` accessors) and signs/verifies it through `SignStruct` / `VerifyInto[T]`. Embedding is enforced at compile time, so a
struct that forgets `ClaimsBase` will not satisfy the generic constraint:

```go
type MyClaims struct {
    jwt.ClaimsBase
    Role   string   `json:"role"`
    Groups []string `json:"groups"`
}

base, _ := signer.NewBase(userID, time.Hour)       // registered claims from the signer's config + a random jti
raw, _ := jwt.SignStruct(signer, key, MyClaims{ClaimsBase: base, Role: "admin"})

out, err := jwt.VerifyInto[MyClaims](ctx, verifier, raw) // *MyClaims, after the full verifier pipeline
_ = out.Role
_ = out.Subject       // promoted from ClaimsBase
_ = out.ExpiryTime()
```

`VerifyInto` is sugar over `Verify` + `Decode`, so it inherits every verifier check (issuer, audience, required claims, typ, temporal);
`SignStruct` is sugar over `Sign`. The `aud` claim is typed as `Audience`, which unmarshals from a single string or an array. The
map API (`Claims` / `Decode` / `Get`) stays available — it is what a `KeyResolver` reads for still-unverified claims.

## Security

Verification is fail-closed:

- The algorithm is checked against the allow-list **before** the signature is verified, closing algorithm-confusion attacks. The default
  allow-list is asymmetric only (`EdDSA`, `ES256`, `RS256`); HMAC is intentionally excluded so a symmetric secret can never be confused
  with a public key. Signing enforces the same allow-list.
- A `VerificationKey` may bind itself to one `Algorithm`; the verifier then rejects a token whose algorithm does not match, binding the
  key to a single scheme.
- An `exp` claim is mandatory by default, and temporal claims (`exp` / `nbf` / `iat`) are validated with the configured clock-skew leeway.

## Usage

```go
signer := jwt.NewSigner(jwt.WithIssuer("billing"))
claims, err := signer.NewClaims(tenantID, time.Hour, "files:read")
raw, err := signer.Sign(jwt.SigningKey{KeyID: "k1", Algorithm: jwt.AlgES256, Key: priv}, claims)

verifier := jwt.NewVerifier(resolver, jwt.WithIssuer("billing"), jwt.WithAudiences("api"))
claims, err = verifier.Verify(ctx, raw)
subject, scopes := claims.Subject(), claims.Scopes()
```

## Errors

| Sentinel                 | Meaning                                                                            |
|--------------------------|-----------------------------------------------------------------------------------|
| `ErrTokenInvalid`        | Malformed token, disallowed algorithm, bad signature, or issuer/audience failure  |
| `ErrTokenExpired`        | `exp` is in the past beyond the clock-skew leeway                                  |
| `ErrAlgorithmNotAllowed` | Algorithm is outside the allow-list, unknown to golang-jwt, or unmatched by a key  |
| `ErrClaimMissing`        | A claim required via `WithRequiredClaims` is absent                                |
| `ErrSigningKeyInvalid`   | Signing material cannot produce a verifiable token (e.g. empty key id)            |
| `ErrTokenTypeInvalid`    | typ header does not match the type required via `WithExpectedTokenType`            |

A `KeyResolver`'s own error is propagated unwrapped, so a caller can match it with `errors.Is`.
