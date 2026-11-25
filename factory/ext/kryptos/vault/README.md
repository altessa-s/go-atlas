# factory

```go
import vaultfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos/vault"
```

Maps the `vault` section of the secrets configuration to the go-kryptos [Vault KV provider][provider] for
[`factory/ext/kryptos`](..), which builds only the in-memory provider.

## Usage

```go
manager, err := secretsfactory.New[any](&cfg.Secrets).
    UseProvider(secretsconfig.ProviderVault, vaultfactory.Builder[any](vaultClient)).
    Build(ctx)
```

| Function                      | Description                                                                                                  |
|-------------------------------|--------------------------------------------------------------------------------------------------------------|
| `Builder[T](client)`             | Provider builder for the `vault` type; reads KV through `client`                                             |
| `NewProvider[T](client, cfg)` | Builds the Vault KV provider of `T` secrets from `secretsconfig.Vault`; `ErrClientRequired` without a client |

[provider]: https://github.com/altessa-s/go-kryptos/tree/main/providers/vault
