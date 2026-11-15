# factory

```go
import "github.com/altessa-s/go-atlas/factory/cache/probfilter"
```

Package `factory` provides fluent builders for creating probabilistic filters and their manager from configuration. Both `FilterBuilder` and
`ManagerBuilder` use deferred error accumulation — errors from any step are collected and returned at `Build(ctx)` time.

## Quick Start

```go
// Build a single filter
filter, err := factory.NewFilter("my-filter", cfg.Filter, cfg.Defaults).
    UseLogger(logger).
    UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
    Build(ctx)

// Build a manager with all configured filters
manager, err := factory.NewManager(cfg.ProbabilisticFilter).
    UseLogger(logger).
    UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
    UseCuckooStorage(probfilterconfig.StorageTypeRedis, cuckooredisfactory.Builder(redisClient)).
    Build(ctx)
```

The builders build only the in-memory storages. `redis` uses the function registered with `UseBloomStorage` / `UseCuckooStorage`; without one
`Build` fails naming the package to add. Any other storage type fails `Build` as unknown.

## Supported Filter Types

| Type     | Description                                                                        |
|----------|------------------------------------------------------------------------------------|
| `bloom`  | Bloom filter — space-efficient membership testing with tunable false positive rate |
| `cuckoo` | Cuckoo filter — supports deletion with configurable capacity                       |

## Supported Storage Types

| Type     | Backend    | Requires                                                                                                                                                                                                                                                                                                |
|----------|------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `memory` | In-process | —                                                                                                                                                                                                                                                                                                       |
| `redis`  | Redis      | Bloom: `UseBloomStorage("redis", bloomredisfactory.Builder(client))` from [`factory/cache/probfilter/bloom/storages/redis`](bloom/storages/redis); Cuckoo: `UseCuckooStorage("redis", cuckooredisfactory.Builder(client))` from [`factory/cache/probfilter/cuckoo/storages/redis`](cuckoo/storages/redis) |

---

## FilterBuilder

### Constructor

| Method                           | Description                                                               |
|----------------------------------|---------------------------------------------------------------------------|
| `NewFilter(name, cfg, defaults)` | Creates a `FilterBuilder` for the given filter name, config, and defaults |

### Dependencies

| Method                      | Description                                                                                                                                                                                                                                                                                                                                     |
|-----------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `UseLogger`                 | Sets the logger for the builder and all created components                                                                                                                                                                                                                                                                                      |
| `UseDefaultLogger`          | Sets the logger to `slog.Default()`                                                                                                                                                                                                                                                                                                             |
| `UseBloomStorage`           | Registers the `BloomStorageFunc` of a storage type the builder does not build (`redis`)                                                                                                                                                                                                                                                         |
| `UseCuckooStorage`          | Registers the `CuckooStorageFunc` of a storage type the builder does not build (`redis`)                                                                                                                                                                                                                                                        |
| `UseDataLoader`             | Sets the source a Bloom filter is rebuilt from; required for `rebuildOnStart` / `rebuildCron`                                                                                                                                                                                                                                                   |
| `UseScheduler`              | Sets the `schedtask.Registrar` that runs `rebuildCron` rebuilds of Redis filters (in-memory filters use a local cron)                                                                                                                                                                                                                  |
| `SkipEvictionPolicyCheck`   | Disables the check of the Redis storages that fails `Build` on a server with an `allkeys-*` `maxmemory-policy` (`probfilter.ErrUnsafeEvictionPolicy`); an unreadable policy (CONFIG denied) only logs a warning. Reaches the storage function as `StorageSpec.SkipEvictionPolicyCheck`; `ManagerBuilder` sets it from `skipEvictionPolicyCheck` |
| `TolerateRebuildInProgress` | Lets `Build` return an unpopulated shared filter when its `rebuildOnStart` rebuild is refused because another process is rebuilding it; only for callers that never trust an unpopulated filter (the negcache factory)                                                                                                                          |

### Terminal

| Method       | Description                                                                                                                                      |
|--------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| `Build(ctx)` | Assembles and returns the probabilistic filter; `ctx` bounds the startup I/O (eviction-policy check, initial rebuild, rebuild task registration) |

---

## ManagerBuilder

### Constructor

| Method            | Description                                                          |
|-------------------|----------------------------------------------------------------------|
| `NewManager(cfg)` | Creates a `ManagerBuilder` for the given probabilistic filter config |

### Dependencies

| Method                        | Description                                                                      |
|-------------------------------|----------------------------------------------------------------------------------|
| `UseLogger`                   | Sets the logger for the manager builder and all created components               |
| `UseDefaultLogger`            | Sets the logger to `slog.Default()`                                              |
| `UseBloomStorage`             | Registers the `BloomStorageFunc` of a storage type for every filter              |
| `UseCuckooStorage`            | Registers the `CuckooStorageFunc` of a storage type for every filter             |
| `UseDataLoader(name, loader)` | Sets the rebuild source of the filter `name` (see `FilterBuilder.UseDataLoader`) |
| `UseScheduler`                | Sets the scheduler for `rebuildCron` rebuilds of every filter                    |
| `UseMeter`                    | Sets the metrics collector of the built `probfilter.Manager`                     |

### Terminal

| Method       | Description                                                                                                                |
|--------------|----------------------------------------------------------------------------------------------------------------------------|
| `Build(ctx)` | Assembles and returns the probabilistic filter manager with all configured filters; `ctx` bounds each filter's startup I/O |

---

## Storage registration

| Type                | Description                                                                                                   |
|---------------------|---------------------------------------------------------------------------------------------------------------|
| `BloomStorageFunc`  | `func(ctx, BloomStorageSpec) (bloomstorages.Storage, error)`; builds the Bloom storage of one storage type    |
| `CuckooStorageFunc` | `func(ctx, CuckooStorageSpec) (cuckoostorages.Storage, error)`; builds the Cuckoo storage of one storage type |
| `StorageSpec`       | Filter name, per-filter `storage` block, logger and `SkipEvictionPolicyCheck`                                 |
| `BloomStorageSpec`  | `StorageSpec` plus `ExpectedItems` and the resolved `FalsePositiveRate`                                       |
| `CuckooStorageSpec` | `StorageSpec` plus `Capacity` and the resolved `CapacityMultiplier`                                           |

A later registration of the same type replaces the earlier one; a nil function fails `Build`.

---

## Configuration semantics

| Setting                     | Effect                                                                                                                                                                                                                                                                                              |
|-----------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `bloom.rebuildOnStart`      | With a data loader: rebuilt synchronously inside `Build` (failure closes the filter and fails `Build`, also `probfilter.ErrRebuildInProgress` from a peer rebuilding the shared filter unless `TolerateRebuildInProgress` is set). Inert without a loader.                                          |
| `bloom.rebuildCron`         | With a data loader: an in-memory filter is rebuilt by a process-local cron; a Redis filter is registered as task `probfilter-rebuild-<name>` with the scheduler (logged as ignored without one). Empty disables it; an invalid cron fails `Build`. Once the filter is closed, both stop rebuilding. |
| `cuckoo.capacityMultiplier` | Redis storage: RedisBloom `EXPANSION` (rounded up). Memory storage cannot grow; a per-filter value is logged as ignored.                                                                                                                                                                            |
