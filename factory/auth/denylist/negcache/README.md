# factory

```go
import "github.com/altessa-s/go-atlas/factory/auth/denylist/negcache"
```

Package `factory` provides a fluent builder that constructs a [`negcache.Cache`](../../../../auth/denylist/negcache/) from a probabilistic-filter
configuration plus injected dependencies. It builds the negative filter through the shared [probfilter factory](../../../cache/probfilter) and wires it
to a caller-supplied `negcache.Authoritative` revocation store. Errors from any step are collected and returned at `Build(ctx)` time.

The factory never builds the authoritative store or a Redis filter storage — following the inject-deps-at-the-consumer convention, the caller injects
the store (for example an `auth/denylist/storages/redis.Store`, which satisfies `negcache.Authoritative`), registers the Redis filter storage with
`UseBloomStorage` / `UseCuckooStorage`, and the factory only assembles the cache around them.

## Quick Start

```go
cache, err := factory.NewBuilder("denylist", cfg.Filter, &defaults, redisDenylist).
    UseLogger(logger).
    UseBloomStorage(probfilterconfig.StorageTypeRedis, // only for a Redis-backed negative filter
        bloomredisfactory.Builder(redisClient)).
    UseScheduler(scheduler). // runs rebuildCron of a Redis-backed filter
    UseMetrics(collector, "auth_denylist_negcache").
    Build(ctx)
defer cache.Close(ctx) // stops the filter's scheduled rebuilds
```

## Rebuilds

The probfilter factory owns the negative filter's rebuilds, driven by the authoritative store: the loader is the one set with `UseDataLoader`, else the
authoritative store itself when it implements `probfilter.DataLoader` (`auth/denylist/storages/redis.Store` does).

| Setting                | Effect with a loader                                                                                                                                                                                                                                            |
|------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `bloom.rebuildOnStart` | Rebuilt inside `Build`; the cache is returned populated. A loader error fails `Build`. If another process is rebuilding a shared Redis filter at that moment, `Build` succeeds and the cache defers to the authoritative store until that rebuild is committed. |
| `bloom.rebuildCron`    | In-memory filter: process-local cron. Redis filter: task `probfilter-rebuild-<name>` registered with the `UseScheduler` scheduler (logged as ignored without one).                                                                                              |

A periodic rebuild is scheduled by default: the Bloom defaults carry `rebuildCron: "0 0 * * * *"` (hourly) and `rebuildOnStart: true`. Between rebuilds
a revocation made on another node is not in this node's filter and is fast-pathed as not revoked, so the cron is the revocation-propagation window —
tighten it to your SLA. `Build` logs a warning (`negcache: no periodic filter rebuild is scheduled`, with a `reason` attribute) whenever nothing is
scheduled: no data loader, an empty `rebuildCron`, a Cuckoo filter, or a Redis filter without `UseScheduler`. The builder's default logger discards
output, so wire `UseLogger` to see it. In those setups rebuild through `cache.Rebuild` on your own schedule.

**Revocation propagation lag.** The hourly default is the generic probabilistic-filter default (`probabilisticFilter.defaults.bloom.rebuildCron`),
shared with every other filter; the denylist has no default of its own. With an in-memory filter a token revoked on another node therefore keeps
passing on this node for up to one hour. Set `denylist.filter.bloom.rebuildCron` to the revocation-propagation SLA of the deployment (for example
`"0 */5 * * * *"` for five minutes), or use a Redis-backed filter (`filter.bloom.storage.type: redis`): it is shared by every node, so a revocation
recorded with `cache.Add` on one node is visible to the others at once.

Without a loader both settings are inert. A Cuckoo filter has no rebuild settings; rebuild it with `cache.Rebuild`. `cache.Close` stops the scheduled
rebuilds. Do not combine these settings with caller-driven rebuilds (`authconfig.Denylist.RebuildInterval`): validation requires
`filter.bloom.rebuildCron: ""` when `rebuildInterval` is set. `redis.Store` streams only from a single Redis server; with a Cluster or Ring client its
stream fails with `ErrUnsupportedClient`, so inject an exact loader with `UseDataLoader` there.

## Constructor

| Method                                                 | Description                                                                                                                                                              |
|--------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `NewBuilder(name, filterCfg, defaults, authoritative)` | Creates a `Builder` for a cache named `name`, built from `filterCfg`/`defaults` and fronting `authoritative`. The name scopes the probfilter's metrics and storage keys. |

## Dependencies

| Method                            | Required                                  | Description                                                                                                                                                                                                                                                             |
|-----------------------------------|-------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `authoritative` (constructor arg) | yes                                       | The exact revocation store the cache fronts. Injected, never built by the factory.                                                                                                                                                                                      |
| `filterCfg` (constructor arg)     | yes                                       | The probabilistic-filter configuration used to build the negative filter.                                                                                                                                                                                               |
| `UseLogger`                       | no                                        | Sets the logger for the builder and the filter it constructs.                                                                                                                                                                                                           |
| `UseDefaultLogger`                | no                                        | Uses `slog.Default()` as the logger.                                                                                                                                                                                                                                    |
| `UseBloomStorage`                 | only for Redis-backed Bloom filters       | Registers a probfilter `BloomStorageFunc` (`bloomredisfactory.Builder(client)` from [`factory/cache/probfilter/bloom/storages/redis`](../../../cache/probfilter/bloom/storages/redis)), passed through to the probfilter factory. In-memory filters need none. |
| `UseCuckooStorage`                | only for Redis-backed Cuckoo filters      | Registers a probfilter `CuckooStorageFunc` (`cuckooredisfactory.Builder(client)` from [`factory/cache/probfilter/cuckoo/storages/redis`](../../../cache/probfilter/cuckoo/storages/redis)), passed through likewise.                                           |
| `UseDataLoader`                   | no                                        | Rebuild source of the filter; overrides the default (the authoritative store, when it is a `probfilter.DataLoader`).                                                                                                                                                    |
| `UseScheduler`                    | for `rebuildCron` of Redis-backed filters | `schedtask.Registrar` that runs the periodic rebuild task.                                                                                                                                                                                                     |
| `UseMetrics`                      | no                                        | Enables lookup telemetry on the built cache using the given collector and subsystem.                                                                                                                                                                                    |

## Terminal

| Method       | Description                                                                                                                                          |
|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Build(ctx)` | Validates the injected dependencies, builds the negative filter (running its `rebuildOnStart` rebuild), and returns the assembled `*negcache.Cache`. |

## See also

- [`negcache`](../../../../auth/denylist/negcache/) — the negative cache this factory assembles.
- [`factory/cache/probfilter`](../../../cache/probfilter) — the filter builder driven here.
