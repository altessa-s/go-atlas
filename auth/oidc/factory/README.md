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
| `UseTokenCache` | Sets the cache for validated token caching |
| `UseRedisClient` | Sets the Redis client for revocation filter storage |
| `UseRevocationStorage` | Sets a custom revocation storage, bypassing auto-creation |
| `UseRevocationAuthoritative` | Sets the exact store that confirms probabilistic filter hits |

JWKS refresh (`jwks.refreshSchedule`) and revocation sync (`revocation.syncSchedule`) run on the provider's own process-local cron, on
every replica; the builder needs no scheduler. The provider owns the revocation filter's rebuilds (the initial sync plus `syncSchedule`), so the
filter is built without probfilter rebuild scheduling, and `revocation.filter.bloom.rebuildCron` or `rebuildOnStart: true` fails `Build` as a
conflicting second schedule (`rebuildCron: ""` / `rebuildOnStart: false` are accepted).

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the OIDC provider |

## Proxy wiring

`Build(ctx)` materializes `cfg.Proxy` (a [`proxyconfig.Proxy`](../../../config/proxy/proxy.go))
into `httpclient.Option` values via `cfg.Proxy.HTTPClientOptions()`, builds the resilient
client with `httpclient.New` and injects it through `oidc.WithHTTPClient(...)`. A nil/empty `Proxy` block
keeps the default `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` env passthrough.
See the [Proxy guide](../../../docs/proxy.md) for YAML modes and operator
guidance.
