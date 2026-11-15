# factory

```go
import gcpfactory "github.com/altessa-s/go-atlas/factory/ext/kryptos/gcp"
```

Maps the `gcp` section of the secrets configuration to the go-kryptos [GCP Secret Manager provider][provider] for
[`factory/ext/kryptos`](..), which builds only the in-memory provider.

## Usage

```go
manager, err := secretsfactory.New(&cfg.Secrets).
    UseProvider(secretsconfig.ProviderGCP, gcpfactory.Builder()).
    Build(ctx)
```

| Function                   | Description                                                                    |
|----------------------------|--------------------------------------------------------------------------------|
| `Builder()`                | Provider builder for the `gcp` type                                            |
| `NewProvider[T](ctx, cfg)` | Builds the GCP Secret Manager provider of `T` secrets from `secretsconfig.GCP` |

[provider]: https://github.com/altessa-s/go-kryptos/tree/main/providers/gcp
