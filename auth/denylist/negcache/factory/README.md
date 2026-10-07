# factory

```go
import "github.com/altessa-s/go-atlas/auth/denylist/negcache/factory"
```

Package `factory` provides a fluent builder that constructs a [`negcache.Cache`](../) from a probabilistic-filter configuration plus injected
dependencies. It builds the negative filter through the shared [probfilter factory](../../../../data/probfilter/factory) and wires it to a
caller-supplied `negcache.Authoritative` revocation store. Errors from any step are collected and returned at `Build()` time.

The factory never builds the authoritative store or a Redis client — following the inject-deps-at-the-consumer convention, the caller injects
those (for example an `auth/denylist/storages/redis.Store`, which satisfies `negcache.Authoritative`) and the factory only assembles the cache
around them.

## Quick Start

```go
cache, err := factory.NewBuilder("denylist", cfg.Filter, &defaults, redisDenylist).
    UseLogger(logger).
    UseRedisClient(redisClient). // only for a Redis-backed negative filter
    UseScheduler(scheduler).     // runs rebuildCron of a Redis-backed filter
    UseMetrics(collector, "auth_denylist_negcache").
    Build()
defer cache.Close(ctx) // stops the filter's scheduled rebuilds
```

## Rebuilds

The probfilter factory owns the negative filter's rebuilds, driven by the authoritative store: the loader is the one set with `UseDataLoader`, else
the authoritative store itself when it implements `probfilter.DataLoader` (`auth/denylist/storages/redis.Store` does).

| Setting | Effect with a loader |
|---------|----------------------|
| `bloom.rebuildOnStart` | Rebuilt inside `Build`; the cache is returned populated. A loader error fails `Build`. If another process is rebuilding a shared Redis filter at that moment, `Build` succeeds and the cache defers to the authoritative store until that rebuild is committed. |
| `bloom.rebuildCron` | In-memory filter: process-local cron. Redis filter: task `probfilter-rebuild-<name>` registered with the `UseScheduler` scheduler (logged as ignored without one). |

Without a loader both settings are inert. A Cuckoo filter has no rebuild settings; rebuild it with `cache.Rebuild`. `cache.Close` stops the
scheduled rebuilds. Do not combine these settings with caller-driven rebuilds (`authconfig.Denylist.RebuildInterval`): validation requires
`filter.bloom.rebuildCron: ""` when `rebuildInterval` is set. `redis.Store` streams only from a single Redis server; with a Cluster or Ring client
its stream fails with `ErrUnsupportedClient`, so inject an exact loader with `UseDataLoader` there.

## Constructor

| Method | Description |
|--------|-------------|
| `NewBuilder(name, filterCfg, defaults, authoritative)` | Creates a `Builder` for a cache named `name`, built from `filterCfg`/`defaults` and fronting `authoritative`. The name scopes the probfilter's metrics and storage keys. |

## Dependencies

| Method | Required | Description |
|--------|----------|-------------|
| `authoritative` (constructor arg) | yes | The exact revocation store the cache fronts. Injected, never built by the factory. |
| `filterCfg` (constructor arg) | yes | The probabilistic-filter configuration used to build the negative filter. |
| `UseLogger` | no | Sets the logger for the builder and the filter it constructs. |
| `UseRedisClient` | only for Redis-backed filters | Redis client passed through to the probfilter factory. In-memory filters need none. |
| `UseDataLoader` | no | Rebuild source of the filter; overrides the default (the authoritative store, when it is a `probfilter.DataLoader`). |
| `UseScheduler` | for `rebuildCron` of Redis-backed filters | `core/scheduler.TaskRegistrar` that runs the periodic rebuild task. |
| `UseMetrics` | no | Enables lookup telemetry on the built cache using the given collector and subsystem. |

## Terminal

| Method | Description |
|--------|-------------|
| `Build` | Validates the injected dependencies, builds the negative filter (running its `rebuildOnStart` rebuild), and returns the assembled `*negcache.Cache`. |

## See also

- [`negcache`](../) — the negative cache this factory assembles.
- [`data/probfilter/factory`](../../../../data/probfilter/factory) — the filter builder driven here.
