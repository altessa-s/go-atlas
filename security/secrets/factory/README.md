# factory

```go
import "github.com/altessa-s/go-atlas/security/secrets/factory"
```

Package `factory` provides configuration-based creation of secret managers and providers. Integrates with
Vault, GCP Secret Manager, Yandex Cloud Lockbox, and in-memory storage.

## Usage

```go
f := factory.New(factory.WithLogger(logger))
mgr, err := f.CreateManagerFromConfig(ctx, &cfg.Secrets)
if err != nil {
    return err
}
defer mgr.Shutdown()
```
