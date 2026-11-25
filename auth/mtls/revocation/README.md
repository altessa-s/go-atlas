# revocation

```go
import "github.com/altessa-s/go-atlas/auth/mtls/revocation"
```

Live OCSP peer-revocation as an [`auth/mtls`](../) `CertValidator`. A `Checker` queries the issuer's OCSP responder for the peer's leaf certificate,
caches the answer until its NextUpdate, retries only transient failures (network errors / HTTP 5xx) with backoff, deduplicates concurrent misses for
the
same certificate (singleflight), and rejects a revoked peer with `coremtls.ErrRevoked`. It is the network-backed counterpart to the in-memory
`coremtls.RevocationList`.

## Options

| Option                                 | Description                                                                                                  |
|----------------------------------------|--------------------------------------------------------------------------------------------------------------|
| `WithFailMode(m)`                      | `FailClosed` (default) rejects an indeterminate result; `FailOpen` accepts it (explicit opt-in).             |
| `WithAllowedResponders(hosts…)`        | Contact only these responder hosts (`host` or `host:port`); lifts the non-public address refusal for them.   |
| `WithInsecureAllowPrivateResponders()` | Let the default client reach a loopback, private or otherwise non-public responder. Exposes SSRF; see below. |
| `WithHTTPClient(c)`                    | HTTP client for the OCSP POST; replaces the default (timeout, same-host redirects, address refusal).         |
| `WithTimeout(d)`                       | Bounds a single check across all attempts (default 5s).                                                      |
| `WithAttemptTimeout(d)`                | Bounds one OCSP request (default 1s); capped to `WithTimeout`. A stalled request costs one attempt.          |
| `WithMaxAttempts(n)`                   | Retries after the first OCSP request, with exponential backoff (default 3, i.e. up to 4 requests).           |
| `WithMaxTTL(d)`                        | Caps how long a status is cached, regardless of NextUpdate (default 1h).                                     |
| `WithMaxCacheEntries(n)`               | Bounds the status cache (default 4096); on overflow it sweeps expired entries, then evicts one.              |
| `WithClockSkew(d)`                     | Tolerance on a response's thisUpdate/nextUpdate and on a delegated responder's validity (default 5m).        |
| `WithMaxResponseAge(d)`                | Max age from thisUpdate of a response without nextUpdate; also caps its caching (default 24h).               |
| `WithClock(now)`                       | Time source override (tests).                                                                                |

## API

| Symbol                | Description                                                                                        |
|-----------------------|----------------------------------------------------------------------------------------------------|
| `New(issuers, opt…)`  | `*Checker` over the trusting issuer CA certificates.                                               |
| `Checker.Validator()` | The checker as a `coremtls.CertValidator`.                                                         |
| `Checker.Check(ctx, leaf)` | Check one leaf: `coremtls.ErrRevoked` when revoked; an indeterminate result follows the fail mode. |

Under `FailClosed` (the default) an indeterminate result returns a wrapped error — `ErrNoIssuer`, `ErrNoResponder`, `ErrResponderNotAllowed`,
`ErrPrivateResponder`, `ErrUnauthorizedResponder`, `ErrUnknownStatus`, `ErrStaleResponse`, `ErrCrossHostRedirect`, or the network failure (match with
`errors.Is`); under `FailOpen` it returns nil.

## Responder destination

> **Private responders are refused by default.** The responder URL is read from the peer's certificate (authorityInfoAccess), so a certificate with
> a chosen URL could otherwise make the server send requests to internal services. A responder that resolves to a loopback, private (RFC 1918),
> link-local, unique-local, multicast, unspecified or otherwise non-public address fails the check with `ErrPrivateResponder`. An internal PKI whose
> responder lives on a private network must name it: `revocation.WithAllowedResponders("ocsp.corp.example")`.

The checker applies three restrictions to the responder URL:

- **Scheme.** Only `http` and `https` URLs are requested; anything else is `ErrResponderNotAllowed`. This also applies with `WithHTTPClient`.
- **Allowlist.** With `WithAllowedResponders(hosts…)` only the listed hosts are contacted; an entry is a host name or IP literal, optionally with a
  port (`ocsp.corp.example`, `ocsp.corp.example:8080`), compared case-insensitively with the URL host. Any other responder is
  `ErrResponderNotAllowed` and no request is sent. Naming a host vouches for it, so the address refusal below does not apply to listed hosts. This
  also applies with `WithHTTPClient`.
- **Address.** Without an allowlist the default client refuses a non-public address (`ErrPrivateResponder`). The refusal runs in the dialer, on the
  resolved IP address right before the connect, so it covers every redirect hop and a host name that resolves — or is rebound — to an internal
  address. It is not retried. Proxy environment variables are ignored while it is active, because a proxied connection would be checked against the
  proxy's address. `WithInsecureAllowPrivateResponders()` turns the refusal off for every responder; prefer the allowlist.

Upgrading: a deployment that checks revocation against a responder on a private address starts failing closed with `ErrPrivateResponder` until the
responder host is listed with `WithAllowedResponders` (or the refusal is turned off).

## Responder authorization

A response is accepted when it is signed with the issuer's own key, or by a delegated responder certificate that the issuer issued directly, that
carries the `id-kp-OCSPSigning` extended key usage and that is within its validity period (RFC 6960 section 4.2.2.2). A response signed by any other
certificate of the same issuer — one without that key usage (`anyExtendedKeyUsage` does not count), expired or not yet valid — is
`ErrUnauthorizedResponder`, whatever status it carries; it is neither retried nor cached.

## Issuers

An OCSP request is keyed by the issuer's name and public key, which the single-leaf validator seam does not carry — so the trusting issuer
certificates
are passed to `New`. In mTLS these are the same CA certificates configured as `ClientCAs`/`RootCAs`. A leaf is matched to its issuer by Authority Key
ID across the whole list, so during a CA key rollover an old CA certificate without a Subject Key ID does not shadow the new one sharing its
subject DN. Only when no key ID decides is the leaf matched by issuer DN, and among several issuers with that DN the one whose key verifies the
leaf's signature is chosen. A cached status is keyed by the issuer's name, the issuer's public key and the serial number, so two CAs that
share a subject DN never share a cache entry.

## Usage

```go
checker := revocation.New(caCerts) // fail-closed; public responders only
// Internal PKI: revocation.New(caCerts, revocation.WithAllowedResponders("ocsp.corp.example"))

authFn, err := grpcmtls.AuthFunc(
    coremtls.WithTrustDomains("example.org"), // the trust decision; a validator is not one
    coremtls.WithValidator(checker.Validator()),
)
```

## Timeouts and retries

`WithTimeout` (default 5s) is the budget of a whole check; `WithAttemptTimeout` (default 1s, never above the check timeout) bounds each request.
With the defaults the first request and the three retries fit into the budget even when every request stalls, so one slow answer does not fail
the check — and, through the shared flight, every concurrent caller with it. Only transient failures — network errors, timeouts and HTTP 5xx —
are retried against the same responder.

A certificate may name several responders. Every URL that passes the destination restrictions above is a candidate: attempts rotate through them
in certificate order, and a certificate naming more responders than `WithMaxAttempts` allows still has each tried once. A URL that is refused is
skipped; when all are refused the check reports the refusal of the first. A responder that fails to give a definitive answer for any other
reason — HTTP 4xx, an unparseable response, a refused connection or redirect, an unauthorized signer, a stale `good`, an `unknown` status — is
dropped and the next one is asked, so it cannot hide a revocation another responder reports. `revoked` from any responder is final. When no
responder answers definitively the error joins every responder's failure (each matches with `errors.Is`).

A blank `WithAllowedResponders` list (entries are trimmed, lower-cased and empty ones dropped) is no allowlist: the default address refusal
applies.

## Fail mode

`FailClosed` is the default: anything the checker cannot confirm good is rejected, so blocking the responder does not let a revoked certificate
through. `FailOpen` keeps new connections flowing during a responder outage at the cost of trusting a peer whose status is unknown; select it
explicitly with `WithFailMode(revocation.FailOpen)` only when an outage costs more than accepting a revoked peer.

## HTTP client

The default client is bounded by the check timeout (`WithTimeout`) and follows a redirect only while it stays on the responder's host and port; a
redirect to a different host fails the query with `ErrCrossHostRedirect` and is not retried. A client passed to `WithHTTPClient` is used as is — its
timeout, redirect policy and destination restrictions (the non-public address refusal included) are the caller's responsibility.

A response is bound to the leaf (issuer signature and serial number). A `good` response outside its validity window — `nextUpdate` passed or
`thisUpdate` in the future, beyond the clock skew — is indeterminate (`ErrStaleResponse`) and never cached. A `good` response without a `nextUpdate`
never expires on its own, so it is valid only until `thisUpdate` plus `WithMaxResponseAge` (default 24h, plus the clock skew) and is cached no longer
than that: a captured answer cannot be replayed for longer. A signed `revoked` response is honored even when stale, since revocation is permanent.

## Scope

Live OCSP is the per-certificate mechanism that fits the validator shape. CRL polling — a periodic bulk fetch rather than a per-peer query — is a
separate concern left for a future addition. The server-side stapler in
[`go-tlsutils/ocsp`](https://github.com/altessa-s/go-tlsutils/tree/develop/ocsp) staples the server's own status and is unrelated to checking a peer.
