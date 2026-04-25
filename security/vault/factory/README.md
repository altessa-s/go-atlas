# factory

```go
import "github.com/altessa-s/go-atlas/security/vault/factory"
```

Package `factory` provides a fluent builder for creating Vault clients from configuration.
`VaultBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
v, err := factory.New(cfg.Vault).
    UseLogger(logger).
    UseTlsConfig(tlsConfig).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

## Supported Auth Methods

| Method    | Config field        | Description                              |
|-----------|---------------------|------------------------------------------|
| Token     | `Auth.Token`        | Static token authentication              |
| AppRole   | `Auth.Approle`      | Role ID and Secret ID credential pair    |
| UserPass  | `Auth.Userpass`     | Username and password authentication     |

## Methods

### Constructor

| Method     | Description                                                  |
|------------|--------------------------------------------------------------|
| `New(cfg)` | Creates a `VaultBuilder` for the given Vault config          |

### Dependencies

| Method                 | Description                                                     |
|------------------------|-----------------------------------------------------------------|
| `UseLogger`            | Sets the logger for the builder and all created components      |
| `UseTlsConfig`         | Sets the TLS configuration for the Vault HTTP client            |
| `UseHealthCoordinator` | Sets the health coordinator for the Vault client                |

### Terminal

| Method       | Description                                                              |
|--------------|--------------------------------------------------------------------------|
| `Build(ctx)` | Assembles and returns the `*vault.Vault` client with the selected auth method |
