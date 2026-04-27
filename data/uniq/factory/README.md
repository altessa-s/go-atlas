# factory

```go
import "github.com/altessa-s/go-atlas/data/uniq/factory"
```

Package `factory` provides a fluent builder for creating a `uniq.Uniq` from configuration.
`UniqBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
u, err := factory.New(cfg.Cache).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

When config is `nil` or storage type is `memory`, a no-op provider is used.

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | No-op (in-process) | — |
| `redis` | Redis | `UseRedisClient` |
| `nats` | NATS KV | `UseNatsConn` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `UniqBuilder` for the given cache storage config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis-backed providers |
| `UseNatsConn` | Sets the NATS connection for NATS-backed providers |
| `UseCollector` | Sets the metrics collector |
| `UseHealthCoordinator` | Auto-registers `*Uniq` with the supplied health coordinator |
| `UseHealthServiceName` | Overrides the health service name (default `"uniq"`) |
| `UseTtl` | Per-key TTL override (wins over `cfg.{Redis,Nats}.Ttl`); zero means "use config or provider default" |

### TTL resolution

For Redis and NATS backends, the per-key TTL is resolved with this precedence:

1. **Per-call** `ttl` argument to `TryAdd(ctx, key, ttl)` / `TryAddWithValue(ctx, key, value, ttl)` if positive
2. `UseTtl(d)` on the builder if called with positive duration
3. `cfg.Redis.Ttl` or `cfg.Nats.Ttl` if non-zero (set in YAML via `storage.redis.ttl` / `storage.nats.ttl`)
4. Provider package default (24 hours for both)

For race-free `TryAdd`-based deduplication, set TTL short enough that a stuck key (panic, kill -9, `Remove` failure) clears before
upstream retries exhaust. Typical value: a small multiple of the broker `AckWait`. Use the per-call argument when one `Uniq` instance
handles keys with different lifetimes; otherwise the config-level default is fine.

> **NATS server requirement.** Per-call TTL on the NATS backend requires server **2.11+**. The provider auto-enables `LimitMarkerTTL` on
> bucket creation; older servers reject the bucket and `New()` fails with `ErrLimitMarkerTTLNotSupported`.

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the unique value manager |
