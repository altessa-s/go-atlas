# factory

```go
import "github.com/altessa-s/go-atlas/security/secrets/factory"
```

Package `factory` provides a fluent builder for creating secrets managers from configuration.
`ManagerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
manager, err := factory.New(cfg.Secrets).
    UseLogger(logger).
    UseVaultClient(vaultClient).
    UseScheduler(scheduler).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

## Supported Providers

| Provider        | Config value | Requires            |
|-----------------|--------------|---------------------|
| Vault KV        | `vault`      | `UseVaultClient`    |
| GCP Secret Manager | `gcp`     | GCP project ID and service account path |
| Yandex Lockbox  | `lockbox`    | Folder ID, key ID, and private key file |
| In-memory       | `memory`     | —                   |

## Methods

### Constructor

| Method     | Description                                                        |
|------------|--------------------------------------------------------------------|
| `New(cfg)` | Creates a `ManagerBuilder` for the given secrets config            |

### Dependencies

| Method                 | Description                                                              |
|------------------------|--------------------------------------------------------------------------|
| `UseLogger`            | Sets the logger for the builder and all created components               |
| `UseVaultClient`       | Sets the Vault client required for the Vault secrets provider            |
| `UseScheduler`         | Sets the task scheduler for background secret refresh                    |
| `UseHealthCoordinator` | Registers the secrets manager with the health system                     |

### Terminal

| Method       | Description                                                              |
|--------------|--------------------------------------------------------------------------|
| `Build(ctx)` | Assembles and returns the `*secrets.Manager[any]`                        |
