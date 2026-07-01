# hmacsign

```go
import "github.com/altessa-s/go-atlas/security/hmacsign"
```

Signs and verifies HTTP request bodies with a shared-secret HMAC — the scheme webhook providers such as Stripe and GitHub use to
prove a request came from them and was not tampered with. It is the body-signing counterpart to the token packages
([`auth/oidc`](../../auth/oidc), [`auth/jwt`](../../auth/jwt)): those authenticate a bearer *token*, this authenticates the *payload*.

A `Verifier` authenticates an inbound webhook; a `Signer` produces the header for an outbound one. Both compare in constant time
(`hmac.Equal`) and use HMAC-SHA256.

## Schemes

A `Scheme` is one provider's wire format. Two are built in:

| Constructor  | Header                | Value                  | Signed message | Timestamp |
|--------------|-----------------------|------------------------|----------------|-----------|
| `GitHub()`   | `X-Hub-Signature-256` | `sha256=<hex>`         | raw body       | no        |
| `Stripe()`   | `Stripe-Signature`    | `t=<unix>,v1=<hex>`    | `<t>.<body>`   | yes       |

Stripe binds a timestamp into the signature, so a stale (replayed) request is rejected by the tolerance window.

`Scheme` is an exported interface — implement `HeaderName`, `Message`, `FormatHeader`, and `ParseHeader` for a provider with a
different grammar (Shopify's base64 body signature, say). A `ParseHeader` that returns the zero time turns the replay window off; the
digest algorithm and secret stay owned by the signer/verifier, so a custom scheme only describes the wire format.

## Options

| Option                       | Applies to        | Description                                                                             |
|------------------------------|-------------------|-----------------------------------------------------------------------------------------|
| `WithSecrets(...[]byte)`     | `Verifier`        | Additional accepted secrets for zero-downtime rotation. Empty secrets are dropped.      |
| `WithTolerance(time.Duration)` | `Verifier`      | Replay window for a timestamped scheme. Default 5m; `0` disables the timestamp check.    |
| `WithClock(Clock)`           | both              | Override the time source (tests pin it; signing stamps it). Default `time.Now`.          |

## Usage

Verify an inbound Stripe webhook:

```go
v := hmacsign.NewVerifier(hmacsign.Stripe(), webhookSecret)

body, _ := io.ReadAll(r.Body) // read the raw bytes once
if err := v.Verify(r.Header.Get(v.HeaderName()), body); err != nil {
    http.Error(w, "bad signature", http.StatusUnauthorized)
    return
}
```

Sign an outbound webhook:

```go
s := hmacsign.NewSigner(hmacsign.Stripe(), webhookSecret)
req.Header.Set(s.HeaderName(), s.Sign(body))
```

Rotate a secret without downtime — verify against the new and old secret at once:

```go
v := hmacsign.NewVerifier(hmacsign.GitHub(), newSecret, hmacsign.WithSecrets(oldSecret))
```

Pass the **exact** bytes that were signed: the HMAC is over the raw body, so any re-encoding (re-marshaling parsed JSON, for example)
breaks the signature.

## Errors

| Sentinel                     | Meaning                                                                              |
|------------------------------|--------------------------------------------------------------------------------------|
| `ErrMalformedSignature`      | The header could not be parsed (missing digest, bad hex, missing/non-numeric `t`).   |
| `ErrTimestampOutOfTolerance` | A timestamped scheme's timestamp fell outside the tolerance window (likely a replay). |
| `ErrSignatureMismatch`       | The header was well-formed but authentic to no configured secret.                    |

Match with `errors.Is`.

## HTTP, config, and factory

- **[`httpsign`](httpsign)** — the `net/http` adapter: `httpsign.Middleware(v)` reads the body once, verifies it, restores it for the
  handler, and rejects unauthentic requests (401). Kept in its own package so this one never imports `net/http`.
- **[`factory`](factory)** + **[`config.WebhookSignature`](../../config)** — build a `Verifier` or `Signer` from a YAML template
  (`scheme`, `secret`, `additionalSecrets`, `tolerance`) with the secret sourced through `config.Secret`.

## See also

- [`auth/jwt`](../../auth/jwt) — mint and verify bearer tokens (authenticates the caller, not the body).
- [`security/secrets`](../secrets) — source the webhook secret from a manager with rotation.
