# factory

```go
import "github.com/altessa-s/go-atlas/data/mongo/kms/factory"
```

Package `factory` provides a fluent builder for creating a MongoDB KMS provider from configuration.
`ProviderBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
provider, err := factory.New(cfg.MongoKMS).
    UseLogger(logger).
    Build()
```

For providers that require mutual TLS, supply a TLS config:

```go
provider, err := factory.New(cfg.MongoKMS).
    UseLogger(logger).
    UseTlsConfig(tlsConfig).
    Build()
```

## Supported KMS Providers

| Provider | Backend | TLS Support |
|----------|---------|-------------|
| `local` | Local master key or key file | — |
| `amazon` | AWS KMS | Optional (`UseTlsConfig`) |
| `azure` | Azure Key Vault | Optional (`UseTlsConfig`) |
| `google` | GCP Cloud KMS | Optional (`UseTlsConfig`) |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ProviderBuilder` for the given KMS config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseTlsConfig` | Sets the TLS configuration for cloud KMS providers |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the KMS provider |
