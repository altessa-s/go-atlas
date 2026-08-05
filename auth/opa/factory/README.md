# factory

```go
import "github.com/altessa-s/go-atlas/auth/opa/factory"
```

Package `factory` provides a fluent builder for creating OPA managers from configuration.
`ManagerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
manager, err := factory.New(cfg.OPA).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseHealthCoordinator(hc).
    Build(ctx)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ManagerBuilder` for the given OPA config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseScheduler` | Sets the task registrar for periodic policy update cycles |
| `UseHealthCoordinator` | Sets the health coordinator for manager health reporting |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the OPA manager |

## Decision cache

`opa.cache` maps onto [`opa.WithDecisionCache`](../README.md#decision-cache):

| Config                 | Effect                                                        |
|------------------------|---------------------------------------------------------------|
| section absent         | Caching off                                                   |
| `enabled: false`       | Caching off                                                   |
| `enabled: true`        | Decisions memoized per policy revision, expiring after `ttl`  |
| `ttl` unset or `0`     | Falls back to `opa.DefaultDecisionCacheTTL` (5m)              |
| `maxSize` unset or `0` | Falls back to `opa.DefaultDecisionCacheSize` (10000 entries)  |

`maxSize` is a memory ceiling, not a correctness knob: the cache is an LRU, so a value too small only evicts sooner and lowers the hit rate. Size it
against the number of distinct inputs seen in one `ttl` window rather than against request volume.

## Proxy wiring

For network-backed policy sources, `Build(ctx)` materializes the per-source
proxy block into client options:

| Source | Config field | Materialized as |
|--------|--------------|-----------------|
| GitLab | `cfg.GitLab.Proxy` ([`config.Proxy`](../../../config/proxy.go)) | `gitlab.WithHTTPClientOptions(httpclient.Option...)` — wires the resilient HTTP client unconditionally |
| S3     | `cfg.S3.Proxy` ([`config.Proxy`](../../../config/proxy.go))     | `awsconfig.WithHTTPClient(httpclient.New(...))` — injected **only** when `Mode` is non-empty, so the AWS SDK keeps its own retry layer otherwise |

A nil/empty `Proxy` block keeps env-based passthrough
(`HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY`). See the
[Proxy guide](../../../docs/proxy.md) for YAML modes and the conditional-
injection rationale for S3.
