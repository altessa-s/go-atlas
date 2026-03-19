# Probabilistic Filters

```go
import "github.com/altessa-s/go-atlas/data/probfilter"
```

Probabilistic filters answer "is X in the set?" using only in-process memory or a shared
Redis bitmap. A negative answer is always correct -- the element is definitely not in the
set. A positive answer may be a false positive, but the rate is configurable (typically
0.1--1%).

The trade-off: a small, tunable false-positive rate in exchange for zero false negatives
and near-zero lookup cost.

---

## Overview

go-atlas provides two filter types with pluggable storage backends (in-memory and Redis).

| Feature               | Bloom               | Cuckoo                          |
|-----------------------|---------------------|---------------------------------|
| Deletion              | No                  | Yes                             |
| Rebuild from source   | Yes (`Rebuild`)     | No                              |
| Memory footprint      | Lower               | Slightly higher                 |
| Full condition        | No                  | Yes (`ErrFilterFull`)           |
| False-positive tuning | `falsePositiveRate` | Fingerprint size (8/12/16 bits) |

Use **Bloom** when you only add items and can periodically rebuild.
Use **Cuckoo** when you need to delete individual items.

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

Filters are configured under the `probabilisticFilter` key in YAML. Shared defaults
reduce repetition; per-filter settings override them.

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
      fingerprintSize: 12
      capacityMultiplier: 2.0
      maxCapacity: 100000000

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

| Field               | Type      | Default       | Description                                   |
|---------------------|-----------|---------------|-----------------------------------------------|
| `storage`           | `string`  | `memory`      | Storage backend: `memory` or `redis`          |
| `expectedItems`     | `int64`   | --            | **Required.** Expected number of items        |
| `falsePositiveRate` | `float64` | `0.01`        | Target false-positive rate (0.0001--0.5)      |
| `rebuildCron`       | `string`  | `0 0 * * * *` | Cron schedule for periodic rebuild            |
| `rebuildOnStart`    | `bool`    | `true`        | Rebuild filter on service startup             |
| `redis`             | `object`  | --            | Redis config (required when `storage: redis`) |

### Cuckoo filter fields

| Field                | Type      | Default     | Description                                   |
|----------------------|-----------|-------------|-----------------------------------------------|
| `storage`            | `string`  | `memory`    | Storage backend: `memory` or `redis`          |
| `capacity`           | `int64`   | --          | **Required.** Initial filter capacity         |
| `fingerprintSize`    | `int`     | `12`        | Fingerprint size in bits (8, 12, 16)          |
| `capacityMultiplier` | `float64` | `2.0`       | Multiplier for auto-resize on overflow        |
| `maxCapacity`        | `int64`   | `100000000` | Maximum allowed capacity                      |
| `redis`              | `object`  | --          | Redis config (required when `storage: redis`) |

---

## Factory builders

The `factory` package creates filters from configuration using a fluent builder API.

### Single filter

```go
import "github.com/altessa-s/go-atlas/data/probfilter/factory"

filter, err := factory.NewFilter("users", filterCfg, defaults).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

### Manager with all configured filters

```go
mgr, err := factory.NewManager(cfg.ProbabilisticFilter).
    UseLogger(logger).
    UseRedisClient(redisClient).
    UseCollector(metricsCollector).
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

// Close all filters that implement io.Closer.
mgr.Close()
```

The Manager accepts a `metrics.Collector` via `WithCollector` option for Prometheus
instrumentation. See [Metrics](#metrics) for details.

---

## Data loading and rebuild

Bloom filters don't support deletion. Instead, rebuild the filter periodically from a
fresh data source using the `DataLoader` interface.

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

When `Count()` returns a positive value, `Rebuild` optimizes by pre-sizing the storage
before streaming values. Otherwise, values are collected in memory first.

### Trigger a rebuild

```go
rebuildable := filter.(*bloom.Filter)
err := rebuildable.Rebuild(ctx, loader)
lastRebuild := rebuildable.LastRebuild()
```

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
| `LastRebuild`       | `time.Time` | Time of last rebuild (Bloom only)          |
| `StorageType`       | `string`    | Backend identifier (`"memory"`, `"redis"`) |

---

## Metrics

When a `metrics.Collector` is provided to the Manager, the following Prometheus metrics
are recorded under the `probfilter` subsystem:

| Metric                                | Type      | Labels                  | Description                    |
|---------------------------------------|-----------|-------------------------|--------------------------------|
| `probfilter_lookups_total`            | Counter   | `filter_name`, `result` | Filter lookups                 |
| `probfilter_adds_total`               | Counter   | `filter_name`           | Items added                    |
| `probfilter_lookup_duration_seconds`  | Histogram | `filter_name`           | Lookup duration                |
| `probfilter_rebuild_duration_seconds` | Histogram | --                      | Rebuild duration               |
| `probfilter_rebuild_errors_total`     | Counter   | --                      | Failed rebuild operations      |

---

## Storage backends

|                | Memory                                       | Redis                           |
|----------------|----------------------------------------------|---------------------------------|
| Latency        | Nanoseconds                                  | Network round-trip              |
| Persistence    | Process lifetime                             | Survives restarts               |
| Sharing        | Single process                               | Multiple processes              |
| Bloom options  | `WithExpectedItems`, `WithFalsePositiveRate` | Same + `WithKeyPrefix`          |
| Cuckoo options | `WithCapacity`                               | `WithCapacity`, `WithKeyPrefix` |
| Requirement    | None                                         | `redis.UniversalClient`         |

Redis implementations batch items into chunks of 1000 for `AddBatch` to avoid
oversized Redis commands.

---

## API reference

### Interfaces

| Interface           | Package              | Methods                                                                  |
|---------------------|----------------------|--------------------------------------------------------------------------|
| `Filter`            | `probfilter`         | `Add`, `AddBatch`, `MightExist`, `Close`                                 |
| `DeletableFilter`   | `probfilter`         | `Filter` + `Delete`                                                      |
| `RebuildableFilter` | `probfilter`         | `Filter` + `Rebuild`, `LastRebuild`                                      |
| `StatsProvider`     | `probfilter`         | `Stats`                                                                  |
| `DataLoader`        | `probfilter`         | `StreamValues`, `Count`                                                  |
| `Manager`           | `probfilter`         | `Register`, `Get`, `MustGet`, `Unregister`, `Names`, `Filters`, `Close`  |

### Builders

| Builder          | Package              | Methods                                                                |
|------------------|----------------------|------------------------------------------------------------------------|
| `FilterBuilder`  | `probfilter/factory` | `NewFilter` -> `UseLogger`, `UseRedisClient`, `Build`                  |
| `ManagerBuilder` | `probfilter/factory` | `NewManager` -> `UseLogger`, `UseRedisClient`, `UseCollector`, `Build` |

### Errors

| Error                    | Package                  | Condition                                     |
|--------------------------|--------------------------|-----------------------------------------------|
| `ErrFilterNotFound`      | `probfilter`             | `Manager.Get` with an unregistered name       |
| `ErrFilterAlreadyExists` | `probfilter`             | `Manager.Register` with a duplicate name      |
| `ErrFilterFull`          | `cuckoo/storages/memory` | Cuckoo filter capacity exhausted              |
