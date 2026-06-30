# factory

```go
import "github.com/altessa-s/go-atlas/auth/mtls/factory"
```

Turns a [`config.MTLS`](../../../config) into the [`auth/mtls`](../) validator options it describes, giving the mTLS subsystem the same
config→component path the [OPA](../../opa/factory) and [scope](../../scope/factory) factories provide.

Only the certificate-validation policy is declarative — the expiry re-check and the trust-domain pin. The identity function, audit recorder,
and transport label stay at the call site, so `Options` returns options a transport adapter combines with its own.

## API

| Symbol                          | Description                                                                              |
|---------------------------------|------------------------------------------------------------------------------------------|
| `New(cfg *config.MTLS)`         | Create a `Builder`. A nil cfg is accepted; the error surfaces at `Options`.               |
| `Builder.Options()`             | `[]coremtls.Option`: expiry validator (when `CheckExpiry`) + trust-domain validator (when `TrustDomains` set). |
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
```

## See also

- [`auth/mtls`](../) — the policy core.
- [`config.MTLS`](../../../config/auth_mtls.go) — the configuration struct.
