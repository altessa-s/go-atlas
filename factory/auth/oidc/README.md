# factory

```go
import "github.com/altessa-s/go-atlas/factory/auth/oidc"
```

Package `factory` provides a fluent builder for creating OIDC providers from configuration. `ProviderBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
provider, err := factory.New(cfg.OIDC).
    UseLogger(logger).
    UseTokenCache(tokenCache).
    UseRevocationBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
    Build(ctx)
```

The revocation filter is built through the [probfilter factory](../../cache/probfilter), which builds only in-memory storages. A `redis`
revocation filter needs the storage registered with `UseRevocationBloomStorage` (from
[`factory/cache/probfilter/bloom/storages/redis`](../../cache/probfilter/bloom/storages/redis)) or `UseRevocationCuckooStorage` (from
[`factory/cache/probfilter/cuckoo/storages/redis`](../../cache/probfilter/cuckoo/storages/redis)); without one `Build` fails naming the package to add.

With custom revocation storage:

```go
provider, err := factory.New(cfg.OIDC).
    UseLogger(logger).
    UseRevocationStorage(customStorage).
    Build(ctx)
```

## Methods

### Constructor

| Method     | Description                                           |
|------------|-------------------------------------------------------|
| `New(cfg)` | Creates a `ProviderBuilder` for the given OIDC config |

### Dependencies

| Method                       | Description                                                                                |
|------------------------------|--------------------------------------------------------------------------------------------|
| `UseLogger`                  | Sets the logger for the builder and all created components                                 |
| `UseDefaultLogger`           | Uses `slog.Default()` as the logger                                                        |
| `UseTokenCache`              | Sets the cache for validated token caching                                                 |
| `UseRevocationBloomStorage`  | Registers the probfilter `BloomStorageFunc` of a revocation filter storage type (`redis`)  |
| `UseRevocationCuckooStorage` | Registers the probfilter `CuckooStorageFunc` of a revocation filter storage type (`redis`) |
| `UseRevocationStorage`       | Sets a custom revocation storage, bypassing auto-creation                                  |
| `UseRevocationAuthoritative` | Sets the exact store that confirms probabilistic filter hits                               |

JWKS refresh (`jwks.refreshSchedule`) and revocation sync (`revocation.syncSchedule`) run on the provider's own process-local cron, on every replica;
the builder needs no scheduler. The provider owns the revocation filter's rebuilds (the initial sync plus `syncSchedule`), so the filter is built
without probfilter rebuild scheduling, and `revocation.filter.bloom.rebuildCron` or `rebuildOnStart: true` fails `Build` as a conflicting second
schedule (`rebuildCron: ""` / `rebuildOnStart: false` are accepted). `revocation.maxStaleness` (default `3h`, `0` disables) maps to
`oidc.WithRevocationMaxStaleness`: with a sync schedule, tokens are rejected once no sync succeeded within it.

| `revocation` key   | Default     | Maps to                                                                                                   |
|--------------------|-------------|-----------------------------------------------------------------------------------------------------------|
| `syncTimeout`      | `5m`        | `oidc.WithRevocationSyncTimeout`; `0` disables the timeout                                                |
| `maxResponseBytes` | `268435456` | `MaxResponseBytes` of the `oidc.URLRevocationLoader` built for `source.url` (256 MiB)                     |
| `allowEmpty`       | `false`     | `oidc.WithRevocationAllowEmpty` when `true`: an empty list empties the filter instead of failing the sync |

`maxResponseBytes` is set on the loader the builder creates, so it has no effect on a storage passed to `UseRevocationStorage`. `allowEmpty` reaches
the provider either way but only the built-in filter storage honors it. With `allowEmpty: true` a source that answers with an empty body during an
outage un-revokes everything.

The context passed to `Build` bounds construction only; the provider's cron outlives its cancellation. The caller owns the returned provider and must
call `provider.Close()` on shutdown, for example from a `runtime.OnShutdown` hook.

### Terminal

| Method  | Description                             |
|---------|-----------------------------------------|
| `Build` | Assembles and returns the OIDC provider |

## Proxy wiring

`Build(ctx)` materializes `cfg.Proxy` (a [`proxyconfig.Config`](../../../config/proxy/proxy.go)) into `httpclient.Option` values via
`proxyfactory.HTTPClientOptions(cfg.Proxy)`, builds the resilient client with `httpclient.New` and injects it through `oidc.WithHTTPClient(...)`. A
nil/empty `Proxy` block keeps SSRF protection enabled and environment proxies disabled. See the [Proxy guide](../../../docs/proxy.md) for YAML modes and
operator guidance.

## Protected outbound clients

Factory-created Atlas HTTP clients use SSRF protection, disable environment proxies and reject explicit proxy modes `url` and `host` with
`httpclient.ErrSSRFUnsafeTransport`. Mode `none` is supported. For a trusted proxy, construct the provider/source directly with an explicitly supplied
client using `httpclient.WithoutSSRFProtection()`; enforce destination restrictions at the proxy.
