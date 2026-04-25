# factory

```go
import "github.com/altessa-s/go-atlas/auth/oidc/factory"
```

Package `factory` provides a fluent builder for creating OIDC providers from configuration.
`ProviderBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
provider, err := factory.New(cfg.OIDC).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseTokenCache(tokenCache).
    UseRedisClient(redisClient).
    Build(ctx)
```

With custom revocation storage:

```go
provider, err := factory.New(cfg.OIDC).
    UseLogger(logger).
    UseRevocationStorage(customStorage).
    Build(ctx)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ProviderBuilder` for the given OIDC config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseScheduler` | Sets the task registrar for background JWKS refresh and revocation sync |
| `UseTokenCache` | Sets the cache for validated token caching |
| `UseRedisClient` | Sets the Redis client for revocation filter storage |
| `UseRevocationStorage` | Sets a custom revocation storage, bypassing auto-creation |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the OIDC provider |

## Proxy wiring

`Build(ctx)` materializes `cfg.Proxy` (a [`config.HTTPProxy`](../../../config/http_proxy.go))
into `httpclient.Option` values via `cfg.Proxy.ClientOptions()` and forwards
them through `oidc.WithHTTPClientOptions(...)`. A nil/empty `Proxy` block
keeps the default `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` env passthrough.
See the [Proxy guide](../../../docs/proxy.md) for YAML modes and operator
guidance.
