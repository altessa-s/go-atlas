# revocation

```go
import "github.com/altessa-s/go-atlas/auth/mtls/revocation"
```

Live OCSP peer-revocation as an [`auth/mtls`](../) `CertValidator`. A `Checker` queries the issuer's OCSP responder for the peer's leaf
certificate, caches the answer until its NextUpdate, retries only transient failures (network errors / HTTP 5xx) with backoff,
deduplicates concurrent misses for the same certificate (singleflight), and rejects a revoked peer with `coremtls.ErrRevoked`. It is the
network-backed counterpart to the in-memory `coremtls.RevocationList`.

## Options

| Option                  | Description                                                                                   |
|-------------------------|-----------------------------------------------------------------------------------------------|
| `WithFailMode(m)`       | `FailOpen` (default) accepts on an indeterminate result; `FailClosed` rejects it.             |
| `WithHTTPClient(c)`     | HTTP client for the OCSP POST.                                                                 |
| `WithTimeout(d)`        | Bounds a single check across all attempts (default 5s).                                        |
| `WithMaxAttempts(n)`    | OCSP attempts with exponential backoff (default 3).                                            |
| `WithMaxTTL(d)`         | Caps how long a status is cached, regardless of NextUpdate (default 1h).                       |
| `WithMaxCacheEntries(n)`| Bounds the status cache (default 4096); on overflow it sweeps expired entries, then evicts one. |
| `WithClock(now)`        | Time source override (tests).                                                                  |

## Issuers

An OCSP request is keyed by the issuer's name and public key, which the single-leaf validator seam does not carry — so the trusting issuer
certificates are passed to `New`. In mTLS these are the same CA certificates configured as `ClientCAs`/`RootCAs`. A leaf is matched to its
issuer by Authority Key ID, falling back to the issuer DN.

## Usage

```go
checker := revocation.New(caCerts, revocation.WithFailMode(revocation.FailClosed))

authFn := grpcmtls.AuthFunc(coremtls.WithValidator(checker.Validator()))
// or combine with other validators (expiry, trust-domain, …).
```

## Fail mode

`FailOpen` keeps new connections flowing during a responder outage at the cost of briefly trusting a peer whose status is unknown;
`FailClosed` rejects anything it cannot confirm good. Choose per the cost of an outage versus the cost of accepting a revoked peer.

## Scope

Live OCSP is the per-certificate mechanism that fits the validator shape. CRL polling — a periodic bulk fetch rather than a per-peer
query — is a separate concern left for a future addition. The server-side stapler in
[`security/tlsutils/ocsp`](../../../security/tlsutils/ocsp) staples the server's own status and is unrelated to checking a peer.
