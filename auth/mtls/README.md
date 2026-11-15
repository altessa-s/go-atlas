# mtls

```go
import "github.com/altessa-s/go-atlas/auth/mtls"
```

Turns a verified mutual-TLS client certificate into an authenticated principal. This is the transport-free policy core behind the
[gRPC](../../transport/grpc/interceptors/auth/mtls) and [HTTP](../../transport/http/server/middlewares/auth/mtls) mTLS adapters: an `Authenticator`
derives the identity, runs extra validators, and records an audit decision. The adapters only extract the verified certificate and map a failure to
their status code.

The default identity is the certificate's SPIFFE ID (via [`auth/spiffe`](../spiffe)); override it with `WithIdentity`.

## Trust decision

The TLS trust pool alone does not say which identities a service accepts: a shared or federated CA can issue a certificate for a foreign SPIFFE trust
domain. `NewAuthenticator` therefore requires an explicit decision — the accepted trust domains pinned with `WithTrustDomains(domains…)`, or the
`WithInsecureAnyTrustDomain()` opt-in. Without either, `NewAuthenticator` returns `ErrNoTrustDomainPolicy` and no authenticator, so a misconfigured
service fails at startup instead of rejecting requests at runtime. The transport adapters return the same error from their constructors.

Validators added with `WithValidator` are ancillary checks and never count as that decision, whatever they check: with an expiry re-check, a
revocation lookup or a `TrustDomainValidator` alone, construction still fails. `WithTrustDomains` trims and lower-cases its domains and drops empty
entries; a call that leaves none sets nothing, so an empty or all-blank list fails the same way. When identity is not a SPIFFE ID (`WithIdentity` reading the Subject DN, classic PKI), `WithTrustDomains`
cannot apply — it requires a SPIFFE ID — so state the decision with `WithInsecureAnyTrustDomain()` and pin the identity with `SubjectValidator`,
`IssuerValidator` or `AuthorityKeyIDValidator`. Pinned domains win over the opt-in when both are set.

## Order of checks

`Authenticate` checks the trust-domain pin first (read from the certificate's URI SAN, independently of `WithIdentity`), then the validators in
order, and calls the identity function last — only for a certificate that passed all of them. An identity function that looks the peer up in a
database or provisions it therefore never runs for a foreign, expired or revoked certificate. The audit record of a rejected certificate carries
the certificate's SPIFFE ID (its Subject DN when it has none); `subjectOf` names accepted principals only.

## Key types

| Type            | Description                                                                                                                                          |
|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Authenticator` | Derives + validates the principal from a verified cert and records an audit decision.                                                                |
| `IdentityFunc`  | `func(*x509.Certificate) (any, error)` — derives the principal. Default `spiffe.IDFromCertificate` (adapted to `IdentityFunc`); opt-in `PrincipalIdentity` yields a `principal.Principal`. |
| `CertValidator` | `func(*x509.Certificate) error` — an extra check run on every call, beyond the TLS handshake.                                                        |

## Functions & options

| Symbol                                  | Description                                                                                                                                    |
|-----------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `NewAuthenticator(opts…)`               | `(*Authenticator, error)`. Default identity `spiffe.IDFromCertificate` (adapted to `IdentityFunc`); `ErrNoTrustDomainPolicy` without `WithTrustDomains` or the any-trust-domain opt-in. |
| `Authenticator.Authenticate(ctx, cert)` | Derive + validate the principal from a verified leaf cert; records audit; fail-closed.                                                         |
| `WithTrustDomains(domains…)`            | The trust decision: accept only these SPIFFE trust domains (`ErrUntrustedDomain` otherwise); checked before the validators.                    |
| `WithInsecureAnyTrustDomain()`          | The explicit opt-out: accept any SPIFFE trust domain, or a certificate without a SPIFFE ID under a custom identity. No effect with pinned domains. |
| `WithIdentity(fn)`                      | Override identity derivation; runs last, for an accepted certificate only. A nil function keeps the default.                                   |
| `WithValidator(v…)`                     | Add ancillary `CertValidator` checks (run in order after the trust-domain pin and before identity); never a trust decision.                    |
| `WithAudit(rec, subjectOf)`             | Record each decision (action `verify_peer_cert`); `subjectOf` names an accepted principal, a rejected certificate is named by its SPIFFE ID or DN. |
| `WithTransport(name)`                   | Label decisions with the driving transport (set by adapters).                                                                                  |
| `ExpiryValidator(now, leeway)`          | Re-check the validity window each call (long-lived connections). `now` defaults to `time.Now`.                                                 |
| `TrustDomainValidator(domains…)`        | The trust-domain pin as a standalone `CertValidator`; without domains it rejects every certificate (`ErrNoTrustDomainPolicy`).                 |
| `RevocationList` / `NewRevocationList`  | Concurrency-safe serial-number denylist; `Revoke`/`Restore`/`IsRevoked`, `Validator()` → `CertValidator`.                                      |
| `SubjectValidator(names…)`              | Pin accepted Subject DNs (classic-PKI identity); subset-match on every set `pkix.Name` field, `Names`/`ExtraNames` included; empty list = any. |
| `IssuerValidator(names…)`               | Pin accepted issuer (CA) DNs; same subset rule, empty list = any.                                                                              |
| `AuthorityKeyIDValidator(keyIDs…)`      | Pin accepted issuing-CA Authority Key IDs; robust CA pin over the verified chain, empty = any.                                                 |
| `DNSNameValidator(names…)`              | Pin accepted DNS SANs via `VerifyHostname` (wildcard-aware, so a wildcard client SAN matches every covered name); empty list = any.            |
| `EKUValidator(ekus…)`                   | Require the cert to assert every given extended key usage (`ExtKeyUsageAny` satisfies all); empty = none.                                      |

## Validation beyond TLS

The TLS handshake already verifies the certificate chain and validity window at connection time. Validators run on **every** call, which matters for
long-lived connections (streams, pooled HTTP/2): `ExpiryValidator` re-checks the validity window so a connection that outlives its certificate is
rejected; `SubjectValidator` / `IssuerValidator` / `AuthorityKeyIDValidator` are the classic-PKI counterpart of the trust-domain pin — pin the
certificate DN, the issuer DN, or the issuing-CA key ID when identity lives in the DN rather than a SPIFFE URI. The DN match
checks every field set in a candidate `pkix.Name` (CN, serial number, O, OU, C, L, ST, street, postal code) and requires each `Names` / `ExtraNames`
attribute in the certificate with the same type and string value; a candidate attribute with a non-string value fails the validator closed with
`ErrUnsupportedDNAttribute`. `DNSNameValidator` pins accepted DNS SANs (wildcard-aware), and `EKUValidator` requires extended key usages (`clientAuth`
on a server verifying clients, `serverAuth` on a client verifying the server). For revocation, `RevocationList` is a built-in concurrency-safe
serial-number denylist — feed it from a CRL poll, an admin action, or a cached OCSP result:

```go
rl := mtls.NewRevocationList()      // or seed: mtls.NewRevocationList("42", "1337")
rl.Revoke(cert.SerialNumber.String())
auth, err := mtls.NewAuthenticator(
    mtls.WithTrustDomains("example.org"),
    mtls.WithValidator(rl.Validator()), // rejects listed serials with ErrRevoked
)
```

The `github.com/altessa-s/go-tlsutils/ocsp` helpers staple the *server's own* certificate status during the handshake; they do not answer "is this
peer certificate revoked". For **live** peer revocation the [`revocation`](./revocation) subpackage queries the issuer's OCSP responder, caches the
result, and returns a `CertValidator` (`revocation.New(caCerts).Validator()`, fail-closed by default) — the network-backed complement to
`RevocationList`. Its default client refuses a responder on a loopback or private address; see the subpackage README before pointing it at an
internal responder.

## Usage

```go
auth, err := mtls.NewAuthenticator(
    mtls.WithTrustDomains("example.org"),
    mtls.WithValidator(mtls.ExpiryValidator(nil, 30*time.Second)),
    mtls.WithAudit(rec, func(p any) string { id, _ := p.(spiffe.ID); return id.String() }),
)
if err != nil {
    return err // ErrNoTrustDomainPolicy: no trust decision
}
principal, err := auth.Authenticate(ctx, verifiedCert)
```

Errors: `ErrNoTrustDomainPolicy`, `ErrNoCertificate`, `ErrCertExpired`, `ErrUntrustedDomain`, `ErrRevoked`, `ErrSubjectMismatch`, `ErrIssuerMismatch`,
`ErrUnsupportedDNAttribute`, `ErrUntrustedCA`, `ErrDNSNameMismatch`, `ErrEKUMissing` (match with `errors.Is`); SPIFFE parse errors propagate from
[`auth/spiffe`](../spiffe).

## See also

- [`auth/spiffe`](../spiffe) — SPIFFE ID parsing.
- [`transport/grpc/interceptors/auth/mtls`](../../transport/grpc/interceptors/auth/mtls) ·
  [`transport/http/server/middlewares/auth/mtls`](../../transport/http/server/middlewares/auth/mtls) — the transport adapters.
- [`factory/auth/mtls`](../../factory/auth/mtls) — build an authenticator's options from `authconfig.MTLS`.
