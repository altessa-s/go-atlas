# factory

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/factory"
```

Package `factory` provides a fluent builder for creating TLS provider registries from configuration.
`ProvidersBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

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
| File           | `TlsProvider.File`  | Certificate and key file paths |
| Vault PKI      | `TlsProvider.Vault` | `UseVaultClient`    |
| Let's Encrypt  | `TlsProvider.LetsEncrypt` | Domain and email  |

## Methods

### Constructor

| Method     | Description                                                          |
|------------|----------------------------------------------------------------------|
| `New(cfg)` | Creates a `ProvidersBuilder` for the given TLS provider config       |

### Dependencies

| Method            | Description                                                           |
|-------------------|-----------------------------------------------------------------------|
| `UseLogger`       | Sets the logger for the builder and all created components            |
| `UseOcspStapler`  | Sets the OCSP stapler attached to file and Vault providers            |
| `UseVaultClient`  | Sets the Vault client required for the Vault TLS provider             |
| `UseCacheDir`     | Sets the cache directory for Vault and Let's Encrypt certificate caching |

### Terminal

| Method    | Description                                                                   |
|-----------|-------------------------------------------------------------------------------|
| `Build()` | Assembles and returns the `*tlsproviders.Providers` registry                  |
