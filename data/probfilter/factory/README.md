# factory

```go
import "github.com/altessa-s/go-atlas/data/probfilter/factory"
```

Package `factory` provides fluent builders for creating probabilistic filters and their manager from configuration.
Both `FilterBuilder` and `ManagerBuilder` use deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
// Build a single filter
filter, err := factory.NewFilter("my-filter", cfg.Filter, cfg.Defaults).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()

// Build a manager with all configured filters
manager, err := factory.NewManager(cfg.ProbabilisticFilter).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

## Supported Filter Types

| Type | Description |
|------|-------------|
| `bloom` | Bloom filter — space-efficient membership testing with tunable false positive rate |
| `cuckoo` | Cuckoo filter — supports deletion with configurable capacity |

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process | — |
| `redis` | Redis | `UseRedisClient` |

---

## FilterBuilder

### Constructor

| Method | Description |
|--------|-------------|
| `NewFilter(name, cfg, defaults)` | Creates a `FilterBuilder` for the given filter name, config, and defaults |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis-backed filter storages |
| `UseDataLoader` | Sets the source a Bloom filter is rebuilt from; required for `rebuildOnStart` / `rebuildCron` |
| `UseScheduler` | Sets the `core/scheduler.TaskRegistrar` that runs `rebuildCron` rebuilds of Redis filters (in-memory filters use a local cron) |
| `SkipEvictionPolicyCheck` | Disables the check that fails `Build` on a Redis server with an `allkeys-*` `maxmemory-policy` (`probfilter.ErrUnsafeEvictionPolicy`); an unreadable policy (CONFIG denied) only logs a warning. `ManagerBuilder` sets it from `skipEvictionPolicyCheck` |
| `TolerateRebuildInProgress` | Lets `Build` return an unpopulated shared filter when its `rebuildOnStart` rebuild is refused because another process is rebuilding it; only for callers that never trust an unpopulated filter (the negcache factory) |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the probabilistic filter |

---

## ManagerBuilder

### Constructor

| Method | Description |
|--------|-------------|
| `NewManager(cfg)` | Creates a `ManagerBuilder` for the given probabilistic filter config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the manager builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis-backed filter storages |
| `UseDataLoader(name, loader)` | Sets the rebuild source of the filter `name` (see `FilterBuilder.UseDataLoader`) |
| `UseScheduler` | Sets the scheduler for `rebuildCron` rebuilds of every filter |
| `UseCollector` | Sets the metrics collector of the built `probfilter.Manager` |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the probabilistic filter manager with all configured filters |

---

## Configuration semantics

| Setting | Effect |
|---------|--------|
| `bloom.rebuildOnStart` | With a data loader: rebuilt synchronously inside `Build` (failure closes the filter and fails `Build`, also `probfilter.ErrRebuildInProgress` from a peer rebuilding the shared filter unless `TolerateRebuildInProgress` is set). Inert without a loader. |
| `bloom.rebuildCron` | With a data loader: an in-memory filter is rebuilt by a process-local cron; a Redis filter is registered as task `probfilter-rebuild-<name>` with the scheduler (logged as ignored without one). Empty disables it; an invalid cron fails `Build`. Once the filter is closed, both stop rebuilding. |
| `cuckoo.capacityMultiplier` | Redis storage: RedisBloom `EXPANSION` (rounded up). Memory storage cannot grow; a per-filter value is logged as ignored. |
| `cuckoo.fingerprintSize` | **Deprecated**, ignored — both backends use 8-bit fingerprints. A per-filter value other than 8 is logged. |
| `cuckoo.maxCapacity` | **Deprecated**, ignored — no backend can bound growth. A per-filter value is logged. |
