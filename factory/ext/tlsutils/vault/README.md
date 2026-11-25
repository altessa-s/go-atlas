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

| Function          | Description                                                                                                  |
|-------------------|--------------------------------------------------------------------------------------------------------------|
| `Builder(client)` | Provider builder for the `vault` type; caches under `<cacheDir>/vault`, `ErrClientRequired` without a client |
