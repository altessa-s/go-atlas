# factory

```go
import "github.com/altessa-s/go-atlas/auth/mtls/factory"
```

Turns an [`authconfig.MTLS`](../../../config/auth) into the [`auth/mtls`](../) validator options it describes, giving the mTLS subsystem the same
config→component path the [OPA](../../opa/factory) and [scope](../../scope/factory) factories provide.

Only the certificate-validation policy is declarative — the expiry re-check, the trust-domain pin, the subject/issuer CommonName and issuing-CA key ID
pins, the DNS-SAN pin, and the required extended key usages. The identity function, audit recorder, and transport label stay at the call site, so
`Options` returns options a transport adapter combines with its own.

## API

| Symbol                          | Description                                                                              |
|---------------------------------|------------------------------------------------------------------------------------------|
| `New(cfg *authconfig.MTLS)`         | Create a `Builder`. A nil cfg is accepted; the error surfaces at `Options`.               |
| `Builder.Options()`             | `[]coremtls.Option`: expiry validator (when `CheckExpiry`) + one validator per non-empty `TrustDomains`, `AllowedSubjectCNs`, `AllowedIssuerCNs`, `IssuerKeyIDs` (hex), `AllowedDNSNames`, `RequiredEKUs` (`clientAuth`/`serverAuth`/`any`). Errors on a nil cfg, bad hex key id, or unknown EKU. |
| `Builder.Authenticator(extra…)` | Standalone `*coremtls.Authenticator` from config + extra options (non-transport callers).  |

## Usage

```go
opts, err := factory.New(&cfg.MTLS).Options()
if err != nil {
    return err
}
// Combine with identity/audit and hand to a transport adapter:
authFn := grpcmtls.AuthFunc(append(opts, coremtls.WithAudit(rec, subjectOf))...)
```

Example configuration:

```yaml
mtls:
  trustDomains: ["example.org"]
  checkExpiry: true
  expiryLeeway: 30s
  allowedSubjectCNs: ["svc-a"]
  allowedIssuerCNs: ["Example Issuing CA"]
  issuerKeyIDs: ["0a1b2c3d"]
  allowedDNSNames: ["svc-a.svc.example.org"]
  requiredEKUs: ["clientAuth"]
```

## See also

- [`auth/mtls`](../) — the policy core.
- [`authconfig.MTLS`](../../../config/auth/auth_mtls.go) — the configuration struct.
