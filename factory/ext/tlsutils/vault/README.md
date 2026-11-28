# factory

```go
import vaultfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils/vault"
```

Maps the `vault` section of the TLS provider configuration to a
[Vault PKI provider](https://github.com/altessa-s/go-tlsutils/tree/develop/providers/vault) for
[`factory/ext/tlsutils`](..), which builds only the file and Let's Encrypt providers.

## Usage

```go
providers, err := tlsfactory.New(cfg.TLSProvider).
    UseProvider(tlsproviders.ProviderTypeVault, vaultfactory.Builder(vaultClient)).
    Build(ctx)
```

| Function                | Description                                                                                                  |
|-------------------------|--------------------------------------------------------------------------------------------------------------|
| `Builder(client, opts…)` | Provider builder for the `vault` type; caches under `<cacheDir>/vault`, `ErrClientRequired` without a client |
| `WithConfiguredNames()` | Serves the configured names only: a handshake without a server name or with a name outside `commonName` and `subjectAlternativeNames` gets the certificate of the common name |
| `WithIssueOnBuild()`    | `Build` obtains the certificate of every configured name before it returns, retrying until its context ends; it fails when Vault issues none or issues one that does not outlive `renewBefore` |

Without options the builder returns the go-tlsutils provider itself, which asks Vault for every server name a client
sends. `WithConfiguredNames` suits a role that issues only its listed names: a probe or a client dialing a Pod IP then
gets the certificate of the common name instead of a failed handshake. With either option the builder returns a wrapper
that keeps the `Provider` and `ClientCertificate` interfaces, not the concrete go-tlsutils type.
