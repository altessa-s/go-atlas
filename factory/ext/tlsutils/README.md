# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
```

Package `factory` provides a fluent builder for creating TLS provider registries from configuration. `ProvidersBuilder` uses deferred error accumulation
— errors from any step are collected and returned at `Build(ctx)` time.

The builder builds the file and Let's Encrypt providers. The Vault and S3 providers register their `ProviderBuilder` with `UseProvider` from their
own packages; a configured section without one fails at `Build` naming the package to add. A builder receives the provider configuration and a
`ProviderEnv` with the shared logger, OCSP stapler, and cache directory.

## Quick Start

```go
providers, err := factory.New(cfg.TLS).
    UseLogger(logger).
    UseProvider(tlsproviders.ProviderTypeVault, vaultfactory.Builder(vaultClient)).
    UseCacheDir("/var/cache/tls").
    Build(ctx)
```

When config is `nil`, `Build` returns an empty `*tlsproviders.Providers` with no error.

If a provider fails, the builder closes the providers it already created and returns any cleanup errors with the build error. After all providers
succeed, it registers the config-built stapler under `ocsp-stapler-refresh-all-<digest>`. The digest identifies the certificate sources, cache
directory and optional namespace. Use `UseOCSPTaskNamespace` with distinct stable names for independent registries that share those inputs and a
scheduler. A scheduler registration error retains lazy refresh and leaves manual refresh available.

## Supported Providers

| Provider      | Config field           | Register                                | Requires                                                         |
|---------------|------------------------|-----------------------------------------|------------------------------------------------------------------|
| File          | `Provider.File`        | built in                                | Certificate and key file paths                                   |
| Vault PKI     | `Provider.Vault`       | [`vaultfactory.Builder(client)`](vault) | A Vault client                                                   |
| Let's Encrypt | `Provider.LetsEncrypt` | built in                                | Domain and email                                                 |
| S3            | `Provider.S3`          | [`s3factory.Builder(client)`](s3)       | Bucket and object keys; a `nil` client is built from the section |

## Methods

### Constructor

| Method     | Description                                                    |
|------------|----------------------------------------------------------------|
| `New(cfg)` | Creates a `ProvidersBuilder` for the given TLS provider config |

### Dependencies

| Method             | Description                                                                                     |
|--------------------|-------------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                                      |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                             |
| `UseOcspStapler`   | Sets the OCSP stapler attached to file, Vault, and S3 providers                                 |
| `UseProvider`      | Registers the `ProviderBuilder` of the Vault or S3 provider type                                |
| `UseCacheDir`      | Sets the cache directory for Vault and Let's Encrypt certificate caching                        |
| `UseScheduler`     | Sets the scheduler for periodic OCSP refresh of a config-built stapler (`ocsp.refreshSchedule`) |

### Terminal

| Method                    | Description                                                                                 |
|---------------------------|---------------------------------------------------------------------------------------------|
| `Build(ctx)`                 | Assembles and returns the `*tlsproviders.Providers` registry                                |

The standalone function `CreateClientConfig(cfg, logger)` builds a client `*tls.Config`. Pass a nil logger to discard warnings.
