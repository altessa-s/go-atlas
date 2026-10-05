# Probabilistic Filters

```go
import "github.com/altessa-s/go-atlas/data/probfilter"
```

Probabilistic filters answer "is X in the set?" using only in-process memory or a shared Redis bitmap. A negative answer is always correct -- the
element is definitely not in the set. A positive answer may be a false positive, but the rate is configurable (typically 0.1--1%).

The trade-off: a small, tunable false-positive rate in exchange for zero false negatives and near-zero lookup cost.

---

## Overview

go-atlas provides two filter types with pluggable storage backends (in-memory and Redis).

| Feature               | Bloom               | Cuckoo                      |
|-----------------------|---------------------|-----------------------------|
| Deletion              | No                  | Yes                         |
| Rebuild from source   | Yes (`Rebuild`)     | Yes (`Rebuild`)             |
| Memory footprint      | Lower               | Slightly higher             |
| Full condition        | No                  | Yes (`ErrFilterFull`)       |
| False-positive tuning | `falsePositiveRate` | Not tunable (backend-fixed) |

Use **Bloom** when you only add items and can periodically rebuild. Use **Cuckoo** when you need to delete individual items; it can be rebuilt from its
source too.

## When to use

| Scenario                              | What the filter prevents                                    | Recommended |
|---------------------------------------|-------------------------------------------------------------|-------------|
| Duplicate event detection             | Redundant DB lookups for already-processed events           | Bloom       |
| Username / email uniqueness pre-check | Unnecessary unique-constraint queries on write path         | Bloom       |
| Cache penetration protection          | Queries for keys that will never exist in the backing store | Bloom       |
| API key pre-validation                | Database hits for clearly-invalid API keys                  | Bloom       |
| Session validation cache              | Round-trips to session store for expired/invalid sessions   | Cuckoo      |
| Blocklist with removal                | Storing and removing blocked IPs/tokens                     | Cuckoo      |

> If most lookups return "not found", probabilistic filters eliminate those round-trips
> entirely with near-zero cost.

---

## Quick start

### Bloom filter

```go
import (
    "github.com/altessa-s/go-atlas/data/probfilter/bloom"
    bloommemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

storage := bloommemory.New(bloommemory.WithExpectedItems(100000))
filter := bloom.New(storage)

_ = filter.Add(ctx, "user:123")
exists, _ := filter.MightExist(ctx, "user:123") // true (maybe)
exists, _ = filter.MightExist(ctx, "user:999")   // false (definitely not)
```

### Cuckoo filter

```go
import (
    "github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
    cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

storage := cuckoomemory.New(cuckoomemory.WithCapacity(100000))
filter := cuckoo.New(storage)

_ = filter.Add(ctx, "session:abc")
removed, _ := filter.Delete(ctx, "session:abc") // true
```

---

## Configuration

Filters are configured under the `probabilisticFilter` key in YAML. Shared defaults reduce repetition; per-filter settings override them.

```yaml
probabilisticFilter:
  defaults:
    bloom:
      storage: memory
      falsePositiveRate: 0.01
      rebuildCron: "0 0 * * * *"
      rebuildOnStart: true
    cuckoo:
      storage: memory
      capacityMultiplier: 2.0   # Redis storage only (RedisBloom EXPANSION)

  filters:
    users:
      type: bloom
      bloom:
        expectedItems: 500000
        falsePositiveRate: 0.001
    sessions:
      type: cuckoo
      cuckoo:
        capacity: 200000
        storage: redis
        redis:
          keysPrefix: "sess:"
```

### Bloom filter fields

| Field               | Type      | Default       | Description                                                       |
|---------------------|-----------|---------------|-------------------------------------------------------------------|
| `storage`           | `string`  | `memory`      | Storage backend: `memory` or `redis`                              |
| `expectedItems`     | `int64`   | --            | **Required.** Expected number of items                            |
| `falsePositiveRate` | `float64` | `0.01`        | Target false-positive rate (0.0001--0.5)                          |
| `rebuildCron`       | `string`  | `0 0 * * * *` | Periodic rebuild; needs a data loader and a scheduler (see below) |
| `rebuildOnStart`    | `bool`    | `true`        | Rebuild on startup; needs a data loader (see below)               |
| `redis`             | `object`  | --            | Redis config (required when `storage: redis`)                     |

### Cuckoo filter fields

| Field                | Type      | Default     | Description                                        |
|----------------------|-----------|-------------|----------------------------------------------------|
| `storage`            | `string`  | `memory`    | Storage backend: `memory` or `redis`               |
| `capacity`           | `int64`   | --          | **Required.** Initial filter capacity              |
| `fingerprintSize`    | `int`     | `12`        | **Deprecated**, ignored (backends use 8 bits)      |
| `capacityMultiplier` | `float64` | `2.0`       | Growth factor; Redis only (`EXPANSION`)            |
| `maxCapacity`        | `int64`   | `100000000` | **Deprecated**, ignored (no backend bounds growth) |
| `redis`              | `object`  | --          | Redis config (required when `storage: redis`)      |

How the `factory` builders apply these settings:

- `rebuildOnStart` / `rebuildCron` need a data source, so they apply only when a loader is injected with `UseDataLoader`; without a loader
  both are inert. `rebuildOnStart` always rebuilds synchronously inside `Build` (a failure fails `Build`), so every new filter is populated
  regardless of persisted schedule state. `rebuildCron` runs where the filter's contents live: an in-memory filter gets a process-local cron
  (a shared, leader-dispatched scheduler would leave other nodes' filters stale); a Redis filter, shared by all nodes, is registered once as
  task `probfilter-rebuild-<name>` with the scheduler from `UseScheduler` (without one the cron is logged as ignored). Once the filter is
  closed, the task and the local cron do nothing.
- `capacityMultiplier` becomes the RedisBloom `EXPANSION` argument (rounded up) of a Redis Cuckoo filter. The memory backend has a fixed-size
  table and does not grow (chained sub-filters would make a delete unable to tell which sub-filter owns a fingerprint). A per-filter value
  with memory storage is logged as ignored.
- `fingerprintSize` and `maxCapacity` are deprecated and ignored: both backends use fixed 8-bit fingerprints and neither can bound growth.
  They are still validated so existing configurations load; per-filter values are logged.

---

## Factory builders

The `factory` package creates filters from configuration using a fluent builder API.

### Single filter

```go
import "github.com/altessa-s/go-atlas/data/probfilter/factory"

filter, err := factory.NewFilter("users", filterCfg, defaults).
    UseLogger(logger).
    UseRedisClient(redisClient).
    UseDataLoader(usersLoader). // enables rebuildOnStart / rebuildCron
    UseScheduler(scheduler).    // runs rebuildCron
    Build()
```

### Manager with all configured filters

```go
mgr, err := factory.NewManager(cfg.ProbabilisticFilter).
    UseLogger(logger).
    UseRedisClient(redisClient).
    UseDataLoader("users", usersLoader).
    UseScheduler(scheduler).
    UseCollector(collector). // probfilter metrics
    Build()
if err != nil {
    log.Fatal(err)
}
defer mgr.Close()
```

---

## Manager

`Manager` is a concurrent-safe registry for named filters.

```go
mgr := probfilter.NewManager()

// Register filters.
mgr.Register("users", usersFilter)
mgr.Register("sessions", sessionsFilter)

// Retrieve by name.
filter, err := mgr.Get("users")
filter = mgr.MustGet("users") // panics if not found

// Iterate.
for name := range mgr.Names() {
    fmt.Println(name)
}
for name, f := range mgr.Filters() {
    fmt.Printf("%s: %T\n", name, f)
}

// Unregister (does not close the filter).
mgr.Unregister("sessions")

// Close all filters (io.Closer or Close(context.Context) error).
mgr.Close()
```

The Manager accepts a `metrics.Collector` via `WithCollector` option for Prometheus instrumentation. See [Metrics](#metrics) for details.

---

## Data loading and rebuild

Bloom filters don't support deletion. Instead, rebuild the filter periodically from a fresh data source using the `DataLoader` interface. Cuckoo filters
can be rebuilt the same way, which also drops fingerprints of values removed from the source but never deleted from the filter.

```go
type DataLoader interface {
    StreamValues(ctx context.Context) iter.Seq2[string, error]
    Count(ctx context.Context) (int64, error)
}
```

### Create a data loader

```go
// From a simple iterator (count unknown).
loader := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
    return func(yield func(string, error) bool) {
        for _, id := range userIDs {
            if !yield(id, nil) { return }
        }
    }
})

// From an iterator with known count (enables optimized rebuild).
loader := probfilter.NewDataLoader(
    func() iter.Seq[string] { return slices.Values(userIDs) },
    probfilter.WithCount(int64(len(userIDs))),
)
```

When `Count()` returns a positive value, `Rebuild` optimizes by pre-sizing the storage before streaming values. Otherwise, values are collected in
memory first.

### Trigger a rebuild

```go
rebuildable := filter.(probfilter.RebuildableFilter) // *bloom.Filter or *cuckoo.Filter
err := rebuildable.Rebuild(ctx, loader)
lastRebuild := rebuildable.LastRebuild()
```

### Rebuild guarantees

`Rebuild` is atomic. The replacement is populated off to the side — a fresh in-process filter, or for Redis a staging key
`__probfilter__:{<tag>}:staging:…` in the live key's cluster slot — while lookups keep seeing the previous contents, and it replaces them in one step (a
pointer swap, or `RENAME`).

- A failed or canceled rebuild (including a loader that stops early on cancellation) aborts the replacement; the previous contents and
  `LastRebuild` stay unchanged.
- Values added through the filter while the rebuild runs are journaled and replayed onto the replacement, so they are present afterwards.
  During a rebuild adds are serialized and the journal holds every add made in that window.
- Cuckoo deletes made during a rebuild apply to the live filter only. If the rebuilt contents contain the value, it reappears as a possible
  member — a false positive, never a false negative. A delete never straddles the commit: it runs entirely against the filter that was live
  when it started.
- A Redis Cuckoo delete is bound to the filter generation it observed: every commit attempt advances a generation counter
  (`__probfilter__:{<tag>}:generation:…`, same cluster slot), and a delete request delayed past a rebuild (for example after a client timeout) finds a
  different token and does nothing instead of removing a colliding member of the new filter (`Delete` then reports an error; it is never retried against
  the new filter). The counter advances before the rename and on every attempt (retries included), so no promotion can keep a generation a delete
  observed.
- Each Redis Cuckoo delete request runs its `CF.DEL` at most once: before the delete, a pending record of the request is stored in a per-filter
  hash without TTL and replaced by the outcome afterwards, so a client retry of the same request after a lost reply returns the outcome — or an
  error when the outcome could not be recorded — instead of removing a second, colliding fingerprint. A request may only execute within one
  minute (server time) of its creation, and records are pruned only after that, so a late retry is rejected too.
- Redis must not evict probfilter keys behind its back: use `noeviction` or a `volatile-*` policy. Filters and delete records carry no TTL;
  only staging keys and commit markers expire, and losing one early only fails a rebuild or makes its outcome indeterminate.
- All Redis metadata lives under the reserved `__probfilter__:` key prefix (hex-encoding the filter key); filter keys in that namespace are rejected
  with `ErrReservedKey`, so metadata never overwrites a filter.
- Rebuilds of one filter are serialized. `Close` interrupts a running rebuild and waits for it; afterwards `Rebuild` returns `ErrFilterClosed`, so no
  rebuild commits after `Close` returns.
- Writes made to a shared Redis filter by **other processes** during the rebuild are not journaled and are lost when the replacement is committed.
- Staging keys are reserved together with a TTL (`StagingTTL`, one hour, refreshed after every staging batch) in one Lua script and are never recreated
  once gone (staging batches use `NOCREATE`), so a process that crashes mid-rebuild cannot leak one; a staging key that vanishes during a slow rebuild
  fails that rebuild. The commit is one Lua script (`RENAME`, `PERSIST`, commit marker).
- Each staging key is created at most once: the stage request records its id in a TTL-less registry and carries a one-minute server-time
  deadline, so a delayed replay of it cannot recreate a staging key that expired or was evicted mid-rebuild.
- If the commit cannot be proven by its marker (no reply, any script error, or neither staging key nor marker found), `Rebuild` returns an error
  wrapping `ErrCommitIndeterminate`: either the previous or the rebuilt contents are in place, and `LastRebuild` is not advanced. The staging key is
  then deleted so a delayed promotion finds nothing to promote; if even that fails, the filter is fenced — lookups, writes and rebuilds fail with
  `ErrCommitIndeterminate`, and `Close` returns it — until deleting the staging key succeeds (retried on every call).
- A batch whose reply reports a per-item failure (for example a full Redis Cuckoo staging filter) fails the rebuild.
- A Redis filter is reserved with its configured parameters before its first write, so RedisBloom never creates it implicitly with default capacity,
  error rate or expansion.
- The Cuckoo replacement is sized for the loaded count plus 25% headroom, never below the configured capacity.

---

## Statistics

Filters that implement `StatsProvider` expose runtime metrics:

```go
sp := filter.(probfilter.StatsProvider)
stats, err := sp.Stats(ctx)
```

| Field               | Type        | Description                                |
|---------------------|-------------|--------------------------------------------|
| `Capacity`          | `int64`     | Maximum items the filter can hold          |
| `ItemCount`         | `int64`     | Approximate items currently stored         |
| `FillRatio`         | `float64`   | Used capacity ratio (0.0--1.0)             |
| `FalsePositiveRate` | `float64`   | Estimated current false-positive rate      |
| `MemoryUsageBytes`  | `int64`     | Memory used (zero for remote backends)     |
| `LastRebuild`       | `time.Time` | Time of last successful rebuild            |
| `StorageType`       | `string`    | Backend identifier (`"memory"`, `"redis"`) |

---

## Metrics

When a `metrics.Collector` is provided to the Manager (`WithCollector`, or `UseCollector` on the factory `ManagerBuilder`), `Register` attaches an
observer to every `ObservableFilter` (the Bloom and Cuckoo filters) that records the following metrics under the `probfilter` subsystem; `Unregister`
and `Close` detach it. Operations on a filter that is not registered with such a Manager are not recorded:

| Metric                                | Type      | Labels                  | Description                                             |
|---------------------------------------|-----------|-------------------------|---------------------------------------------------------|
| `probfilter_lookups_total`            | Counter   | `filter_name`, `result` | Lookups; `result` is `positive`, `negative`, or `error` |
| `probfilter_adds_total`               | Counter   | `filter_name`           | Items successfully added                                |
| `probfilter_lookup_duration_seconds`  | Histogram | `filter_name`           | Lookup duration                                         |
| `probfilter_rebuild_duration_seconds` | Histogram | --                      | Rebuild duration (all rebuilds)                         |
| `probfilter_rebuild_errors_total`     | Counter   | --                      | Failed rebuild operations                               |

---

## Storage backends

|                | Memory                                       | Redis                                            |
|----------------|----------------------------------------------|--------------------------------------------------|
| Latency        | Nanoseconds                                  | Network round-trip                               |
| Persistence    | Process lifetime                             | Survives restarts                                |
| Sharing        | Single process                               | Multiple processes                               |
| Bloom options  | `WithExpectedItems`, `WithFalsePositiveRate` | Same + `WithKeyPrefix`                           |
| Cuckoo options | `WithCapacity`                               | `WithCapacity`, `WithKeyPrefix`, `WithExpansion` |
| Requirement    | None                                         | `redis.UniversalClient`                          |

Redis implementations batch items into chunks of 1000 for `AddBatch` to avoid oversized Redis commands. They accept replies in both RESP2 and RESP3 (the
go-redis v9 default): RedisBloom answers `*.EXISTS`/`CF.DEL` with integers under RESP2 and booleans under RESP3, and `*.INFO` with an array or a map.

The in-memory Cuckoo storage never evicts a member on a failed insert: `ErrFilterFull` leaves the filter unchanged. Deleting a value that was
never added can still remove a colliding member's fingerprint, so only delete values that were added.

---

## API reference

### Interfaces and types

| Interface           | Package      | Methods                                                                 |
|---------------------|--------------|-------------------------------------------------------------------------|
| `Filter`            | `probfilter` | `Add`, `AddBatch`, `MightExist`                                         |
| `DeletableFilter`   | `probfilter` | `Filter` + `Delete`                                                     |
| `RebuildableFilter` | `probfilter` | `Filter` + `Rebuild`, `LastRebuild`                                     |
| `ObservableFilter`  | `probfilter` | `Filter` + `SetObserver`                                                |
| `Observer`          | `probfilter` | `ObserveLookup`, `ObserveAdd`, `ObserveRebuild`                         |
| `StatsProvider`     | `probfilter` | `Stats`                                                                 |
| `DataLoader`        | `probfilter` | `StreamValues`, `Count`                                                 |
| `Manager` (struct)  | `probfilter` | `Register`, `Get`, `MustGet`, `Unregister`, `Names`, `Filters`, `Close` |

### Builders

| Builder          | Package              | Methods                                                                                                                     |
|------------------|----------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `FilterBuilder`  | `probfilter/factory` | `NewFilter` -> `UseLogger`, `UseRedisClient`, `UseDataLoader`, `UseScheduler`, `Build`                                      |
| `ManagerBuilder` | `probfilter/factory` | `NewManager` -> `UseLogger`, `UseDefaultLogger`, `UseRedisClient`, `UseDataLoader`, `UseScheduler`, `UseCollector`, `Build` |

### Errors

| Error                    | Package                  | Condition                                                |
|--------------------------|--------------------------|----------------------------------------------------------|
| `ErrFilterNotFound`      | `probfilter`             | `Manager.Get` with an unregistered name                  |
| `ErrFilterAlreadyExists` | `probfilter`             | `Manager.Register` with a duplicate name                 |
| `ErrFilterClosed`        | `probfilter`             | `Rebuild` on a closed filter                             |
| `ErrCommitIndeterminate` | `probfilter`             | Wrapped when a Redis rebuild commit's outcome is unknown |
| `ErrFilterFull`          | `cuckoo/storages/memory` | Cuckoo filter capacity exhausted                         |
