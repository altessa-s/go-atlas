# mtls

```go
import "github.com/altessa-s/go-atlas/auth/mtls"
```

Turns a verified mutual-TLS client certificate into an authenticated principal. This is the transport-free policy core behind the
[gRPC](../../transport/grpc/interceptors/auth/mtls) and [HTTP](../../transport/http/server/middlewares/auth/mtls) mTLS adapters: an
`Authenticator` derives the identity, runs extra validators, and records an audit decision. The adapters only extract the verified
certificate and map a failure to their status code.

The default identity is the certificate's SPIFFE ID (via [`auth/spiffe`](../spiffe)); override it with `WithIdentity`.

## Key types

| Type            | Description                                                                                     |
|-----------------|-------------------------------------------------------------------------------------------------|
| `Authenticator` | Derives + validates the principal from a verified cert and records an audit decision.            |
| `IdentityFunc`  | `func(*x509.Certificate) (any, error)` — derives the principal. Default `SPIFFEIdentity`.         |
| `CertValidator` | `func(*x509.Certificate) error` — an extra check run on every call, beyond the TLS handshake.     |

## Functions & options

| Symbol                              | Description                                                                                    |
|-------------------------------------|------------------------------------------------------------------------------------------------|
| `NewAuthenticator(opts…)`           | Build an authenticator. Default identity `SPIFFEIdentity`, no validators.                       |
| `Authenticator.Authenticate(ctx, cert)` | Derive + validate the principal from a verified leaf cert; records audit; fail-closed.     |
| `WithIdentity(fn)`                  | Override identity derivation.                                                                   |
| `WithValidator(v…)`                 | Add `CertValidator` checks (run in order after identity).                                       |
| `WithAudit(rec, subjectOf)`         | Record each decision (action `verify_peer_cert`).                                               |
| `WithTransport(name)`               | Label decisions with the driving transport (set by adapters).                                   |
| `ExpiryValidator(now, leeway)`      | Re-check the validity window each call (long-lived connections). `now` defaults to `time.Now`.   |
| `TrustDomainValidator(domains…)`    | Pin accepted SPIFFE trust domains; empty list = any.                                            |
| `RevocationList` / `NewRevocationList` | Concurrency-safe serial-number denylist; `Revoke`/`Restore`/`IsRevoked`, `Validator()` → `CertValidator`. |
| `SubjectValidator(names…)`          | Pin accepted Subject DNs (classic-PKI identity); subset-match on `pkix.Name`, empty list = any.   |
| `IssuerValidator(names…)`           | Pin accepted issuer (CA) DNs; same subset rule, empty list = any.                                |
| `AuthorityKeyIDValidator(keyIDs…)`  | Pin accepted issuing-CA Authority Key IDs; robust CA pin over the verified chain, empty = any.    |
| `DNSNameValidator(names…)`          | Pin accepted DNS SANs via `VerifyHostname` (wildcard-aware); empty list = any.                    |
| `EKUValidator(ekus…)`               | Require the cert to assert every given extended key usage (`ExtKeyUsageAny` satisfies all); empty = none. |

## Validation beyond TLS

The TLS handshake already verifies the certificate chain and validity window at connection time. Validators run on **every** call, which
matters for long-lived connections (streams, pooled HTTP/2): `ExpiryValidator` re-checks the validity window so a connection that outlives
its certificate is rejected; `TrustDomainValidator` pins SPIFFE trust domains, while `SubjectValidator` / `IssuerValidator` /
`AuthorityKeyIDValidator` are the classic-PKI counterpart — pin the certificate DN, the issuer DN, or the issuing-CA key ID when identity
lives in the DN rather than a SPIFFE URI. `DNSNameValidator` pins accepted DNS SANs (wildcard-aware), and `EKUValidator` requires extended
key usages (`clientAuth` on a server verifying clients, `serverAuth` on a client verifying the server). For revocation, `RevocationList` is a built-in concurrency-safe
serial-number denylist — feed it from a CRL poll, an admin action, or a cached OCSP result:

```go
rl := mtls.NewRevocationList()      // or seed: mtls.NewRevocationList("42", "1337")
rl.Revoke(cert.SerialNumber.String())
auth := mtls.NewAuthenticator(mtls.WithValidator(rl.Validator())) // rejects listed serials with ErrRevoked
```

The `security/tlsutils/ocsp` helpers staple the *server's own* certificate status during the handshake; they do not answer "is this peer
certificate revoked". For **live** peer revocation the [`revocation`](./revocation) subpackage queries the issuer's OCSP responder, caches
the result, and returns a `CertValidator` (`revocation.New(caCerts).Validator()`) — the network-backed complement to `RevocationList`.

## Usage

```go
auth := mtls.NewAuthenticator(
    mtls.WithValidator(mtls.ExpiryValidator(nil, 30*time.Second)),
    mtls.WithValidator(mtls.TrustDomainValidator("example.org")),
    mtls.WithAudit(rec, func(p any) string { id, _ := p.(spiffe.ID); return id.String() }),
)
principal, err := auth.Authenticate(ctx, verifiedCert)
```

Errors: `ErrNoCertificate`, `ErrCertExpired`, `ErrUntrustedDomain` (match with `errors.Is`); SPIFFE parse errors propagate from
[`auth/spiffe`](../spiffe).

## See also

- [`auth/spiffe`](../spiffe) — SPIFFE ID parsing.
- [`transport/grpc/interceptors/auth/mtls`](../../transport/grpc/interceptors/auth/mtls) · [`transport/http/server/middlewares/auth/mtls`](../../transport/http/server/middlewares/auth/mtls) — the transport adapters.
- [`auth/mtls/factory`](./factory) — build an authenticator's options from `config.MTLS`.
