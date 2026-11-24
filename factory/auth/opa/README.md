# factory

```go
import "github.com/altessa-s/go-atlas/factory/auth/opa"
```

Package `factory` provides a fluent builder for creating OPA managers from configuration. `ManagerBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

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
| `UseDefaultLogger` | Uses `slog.Default()` as the logger |
| `UseMeter` | Sets the meter the manager records its metrics with |
| `UseScheduler` | Sets the task registrar for periodic policy update cycles |
| `UseHealthCoordinator` | Sets the health coordinator for manager health reporting |
| `UseEmbedFS` | Sets the `fs.FS` and directory for the embed policy source |
| `UseS3Client` | Injects an `s3source.S3API` client for the S3 source instead of building one from config |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the OPA manager |

## Decision cache

`opa.cache` maps onto [`opa.WithDecisionCache`](../../../auth/opa/README.md#decision-cache):

| Config                 | Effect                                                        |
|------------------------|---------------------------------------------------------------|
| section absent         | Caching off                                                   |
| `enabled: false`       | Caching off                                                   |
| `enabled: true`        | Decisions memoized per policy revision, expiring after `ttl`  |
| `ttl` unset or `0`     | Falls back to `opa.DefaultDecisionCacheTTL` (5m)              |
| `maxSize` unset or `0` | Falls back to `opa.DefaultDecisionCacheSize` (10000 entries)  |

`maxSize` is a memory ceiling, not a correctness knob: the cache is an LRU, so a value too small only evicts sooner and lowers the hit rate. Size it
against the number of distinct inputs seen in one `ttl` window rather than against request volume.

## Policy integrity

| Config                     | Materialized as                                                                   |
|----------------------------|-----------------------------------------------------------------------------------|
| `gitlab.allowInsecureHTTP` | `gitlab.WithAllowInsecureHTTP()` — accept a plain `http://` endpoint              |
| `gitlab.checksums`         | `gitlab.WithChecksums(...)` — SHA-256 manifest keyed by file name under `dir`     |
| `s3.allowInsecureHTTP`     | accept a plain `http://` `s3.endpoint`; otherwise `s3source.ErrInsecureEndpoint`  |
| `s3.checksums`             | `s3source.WithChecksums(...)` — SHA-256 manifest keyed by object key under prefix |

An empty or absent `checksums` map disables verification. Digests are lower-case hex; validation rejects any other form. A GitLab or S3 source built
without a manifest loads whatever the remote serves and logs a warning once at construction (`... policy source has no checksums manifest`) through
the builder's logger — the default logger discards it, so wire `UseLogger`.

## Outbound HTTP policy

For network-backed policy sources, `Build(ctx)` translates the per-source `proxy` and `ssrf` blocks into client options.

| Source | YAML blocks | Client selection |
|--------|-------------|------------------|
| GitLab | `opa.gitlab.proxy`, `opa.gitlab.ssrf` | Always uses the Atlas HTTP client via `gitlab.WithHTTPClient(...)`. |
| S3     | `opa.s3.proxy`, `opa.s3.ssrf`         | Uses the Atlas HTTP client when `proxy.mode: none` or an `ssrf` block is set. Otherwise uses the AWS SDK client. |

```yaml
opa:
  gitlab:
    ssrf:
      allowedCIDRs:
        - 10.0.2.0/24
  s3:
    ssrf:
      allowedCIDRs:
        - 10.0.3.0/24
```

Atlas clients use SSRF protection by default. Use `allowedCIDRs` or `allow` rules for a private endpoint. A matching `deny` rule takes precedence.
Set `disabled: true` to turn protection off. Mode `none` always connects directly, and explicit proxy modes `url` and `host` return
`httpclient.ErrSSRFUnsafeTransport`. See the [OPA configuration guide](../../../docs/auth/opa.md#factory) and the [Proxy guide](../../../docs/proxy.md).
