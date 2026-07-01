# factory

```go
import "github.com/altessa-s/go-atlas/security/hmacsign/factory"
```

Builds [`hmacsign`](..) signers and verifiers from a [`config.WebhookSignature`](../../../config) template.

| Function            | Description                                                                                    |
|---------------------|------------------------------------------------------------------------------------------------|
| `Verifier(cfg)`     | Build a `*hmacsign.Verifier` — scheme, primary + rotation secrets, and replay tolerance.        |
| `Signer(cfg)`       | Build a `*hmacsign.Signer` — scheme and the primary secret (rotation/tolerance do not apply).    |

The `scheme` config string (`github` / `stripe`) selects the wire format; the secret is read through `config.Secret`, so
`file:`/`env:`/`vault:` references resolve before the object is built.

## Usage

```go
v, err := factory.Verifier(cfg.WebhookSignature)
if err != nil {
    return err
}
mux.Handle("/webhooks/stripe", httpsign.Middleware(v)(handler))
```

## See also

- [`hmacsign`](..) — the underlying signer/verifier and schemes.
- [`hmacsign/httpsign`](../httpsign) — the `net/http` middleware that consumes the built verifier.
- [`config.WebhookSignature`](../../../config) — the configuration template.
