# factory

```go
import lockboxfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos/lockbox"
```

Maps the `lockbox` section of the secrets configuration to the go-kryptos [Yandex Cloud Lockbox provider][provider] for
[`factory/ext/kryptos`](..), which builds only the in-memory provider.

## Usage

```go
manager, err := secretsfactory.New(&cfg.Secrets).
    UseProvider(secretsconfig.ProviderLockbox, lockboxfactory.Builder()).
    Build(ctx)
```

| Function                   | Description                                                                                           |
|----------------------------|-------------------------------------------------------------------------------------------------------|
| `Builder()`                | Provider builder for the `lockbox` type                                                               |
| `NewProvider[T](ctx, cfg)` | Builds the Lockbox provider of `T` secrets from `secretsconfig.Lockbox`, reading the private key file |

`NewProvider` returns a `*Provider[T]` that owns its service-account token source: `Close` closes both.

[provider]: https://github.com/altessa-s/go-kryptos/tree/main/providers/lockbox
