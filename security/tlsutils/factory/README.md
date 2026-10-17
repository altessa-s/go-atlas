# factory

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/factory"
```

Package `factory` provides a fluent builder for creating TLS provider registries from configuration. `ProvidersBuilder` uses deferred error accumulation
— errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
providers, err := factory.New(cfg.TLS).
    UseLogger(logger).
    UseVaultClient(vaultClient).
    UseCacheDir("/var/cache/tls").
    Build()
```

When config is `nil`, `Build` returns an empty `*tlsproviders.Providers` with no error.

## Supported Providers

| Provider       | Config field        | Requires            |
|----------------|---------------------|---------------------|
| File           | `Provider.File`  | Certificate and key file paths |
| Vault PKI      | `Provider.Vault` | `UseVaultClient`    |
| Let's Encrypt  | `Provider.LetsEncrypt` | Domain and email  |
| S3             | `Provider.S3`    | Bucket and object keys; `UseS3Client` optional (built from config otherwise) |

## Methods

### Constructor

| Method     | Description                                                          |
|------------|----------------------------------------------------------------------|
| `New(cfg)` | Creates a `ProvidersBuilder` for the given TLS provider config       |

### Dependencies

| Method            | Description                                                           |
|-------------------|-----------------------------------------------------------------------|
| `UseLogger`       | Sets the logger for the builder and all created components            |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                  |
| `UseOcspStapler`  | Sets the OCSP stapler attached to file, Vault, and S3 providers           |
| `UseVaultClient`  | Sets the Vault client required for the Vault TLS provider             |
| `UseCacheDir`     | Sets the cache directory for Vault and Let's Encrypt certificate caching |
| `UseS3Client`     | Sets the S3 client for the S3 TLS provider                            |
| `UseScheduler`    | Sets the scheduler for periodic OCSP refresh of a config-built stapler (`ocsp.refreshSchedule`) |

### Terminal

| Method    | Description                                                                   |
|-----------|-------------------------------------------------------------------------------|
| `Build()` | Assembles and returns the `*tlsproviders.Providers` registry                  |
| `CreateClientConfig(cfg)` | Builds a client `*tls.Config` from a `tlsconfig.Client`; independent of the provider config |
