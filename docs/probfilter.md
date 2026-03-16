# Probabilistic Filters

Checking whether an element exists in a large dataset typically requires a database query or network call. When most lookups return "not found", 
these round-trips are wasted work. Probabilistic filters sit in front of the slower data source and answer the question "is X in the set?" using 
only in-process memory or a shared Redis bitmap. A negative answer is always correct — the element is definitely not in the set. A positive answer 
may be a false positive, but the rate is configurable and typically low (0.1–1%). The trade-off: a small, tunable false-positive rate in exchange for 
zero false negatives and near-zero lookup cost.

go-atlas provides two filter types — Bloom and Cuckoo — with pluggable storage backends (in-memory and Redis).

## When to use probabilistic filters

If you need to answer "is X in the set?" before hitting a slower data source, and can tolerate rare false positives, use a probabilistic filter. 
Common use cases in backend services:

| Use case                              | What the filter prevents                                        | Filter type |
|---------------------------------------|-----------------------------------------------------------------|-------------|
| Duplicate event detection             | Redundant DB lookups for already-processed events               | Bloom       |
| Username / email uniqueness pre-check | Unnecessary unique-constraint queries on write path             | Bloom       |
| Cache penetration protection          | Queries for keys that will never exist in the backing store     | Bloom       |
| API key pre-validation                | Database hits for clearly-invalid API keys                      | Bloom       |
| Session validation cache              | Round-trips to session store for expired/invalid sessions       | Cuckoo      |
| Blocklist with removal                | Storing and removing blocked IPs/tokens with individual deletes | Cuckoo      |

Bloom filters are append-only and periodically rebuilt from a data source. Choose Bloom when items are never deleted individually. Cuckoo filters 
support per-item deletion at the cost of slightly higher memory usage. Choose Cuckoo when you need to remove entries without a full rebuild.

## Choose a filter type

| Feature               | Bloom               | Cuckoo                          |
|-----------------------|---------------------|---------------------------------|
| Deletion              | No                  | Yes                             |
| Rebuild               | Yes (`Rebuild`)     | No                              |
| Memory footprint      | Lower               | Slightly higher                 |
| Full condition        | No                  | Yes (`ErrFilterFull`)           |
| False-positive tuning | `falsePositiveRate` | Fingerprint size (8/12/16 bits) |

Use **Bloom** when you only add items and can periodically rebuild. Use **Cuckoo** when you need to delete individual items.

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

## Configure filters

Filters are configured under the `probabilisticFilter` key in YAML. Shared defaults reduce repetition; per-filter settings override them.

```yaml
probabilisticFilter:
  defaults:
    bloom:
      storage: memory          # memory | redis (default: memory)
      falsePositiveRate: 0.01  # 0.0001–0.5 (default: 0.01)
      rebuildCron: "0 0 * * * *" # every hour (default)
      rebuildOnStart: true     # rebuild on startup (default: true)
    cuckoo:
      storage: memory          # memory | redis (default: memory)
      fingerprintSize: 12      # 8, 12, or 16 bits (default: 12)
      capacityMultiplier: 2.0  # 1.1–10.0 (default: 2.0)
      maxCapacity: 100000000   # minimum 1000 (default: 100000000)

  filters:
    users:
      type: bloom
      bloom:
        expectedItems: 500000
        falsePositiveRate: 0.001   # override default
        # storage inherits "memory" from defaults
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
| `storage`           | `string`  | `memory`      | Storage backend (`memory` or `redis`)         |
| `expectedItems`     | `int64`   | —             | **Required.** Expected number of items        |
| `falsePositiveRate` | `float64` | `0.01`        | Target false-positive rate                    |
| `rebuildCron`       | `string`  | `0 0 * * * *` | Cron schedule for periodic rebuild            |
| `rebuildOnStart`    | `bool`    | `true`        | Rebuild filter on service startup             |
| `redis`             | `object`  | —             | Redis config (required when `storage: redis`) |

### Cuckoo filter fields

| Field                | Type      | Default     | Description                                   |
|----------------------|-----------|-------------|-----------------------------------------------|
| `storage`            | `string`  | `memory`    | Storage backend (`memory` or `redis`)         |
| `capacity`           | `int64`   | —           | **Required.** Initial filter capacity         |
| `fingerprintSize`    | `int`     | `12`        | Fingerprint size in bits (8, 12, 16)          |
| `capacityMultiplier` | `float64` | `2.0`       | Multiplier for auto-resize on overflow        |
| `maxCapacity`        | `int64`   | `100000000` | Maximum allowed capacity                      |
| `redis`              | `object`  | —           | Redis config (required when `storage: redis`) |

## Build filters from config

The `factory` package provides fluent builders that create filters from configuration.

### Build a single filter

```go
import "github.com/altessa-s/go-atlas/data/probfilter/factory"

filter, err := factory.NewFilter("users", filterCfg, defaults).
    UseLogger(logger).
    UseRedisClient(redisClient). // only needed for redis storage
    Build()
```

### Build a manager with all configured filters

```go
mgr, err := factory.NewManager(cfg.ProbabilisticFilter).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
if err != nil {
    log.Fatal(err)
}
defer mgr.Close()
```

## Manage multiple filters

`Manager` is a concurrent-safe registry for named filters.

```go
mgr := probfilter.NewManager()

// Register filters
_ = mgr.Register("users", usersFilter)
_ = mgr.Register("sessions", sessionsFilter)

// Retrieve by name
filter, err := mgr.Get("users")
filter = mgr.MustGet("users") // panics if not found

// Iterate
for name := range mgr.Names() {
    fmt.Println(name)
}
for name, f := range mgr.Filters() {
    fmt.Printf("%s: %T\n", name, f)
}

// Unregister (does not close the filter)
mgr.Unregister("sessions")

// Close all filters
_ = mgr.Close()
```

## Load data and rebuild

Bloom filters don't support deletion. Instead, rebuild the filter periodically from a fresh data source using the `DataLoader` interface.

```go
// DataLoader streams values for rebuild.
type DataLoader interface {
    StreamValues(ctx context.Context) iter.Seq2[string, error]
    Count(ctx context.Context) (int64, error)
}
```

### Create a data loader

```go
// From a simple iterator (count unknown)
loader := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
    return func(yield func(string, error) bool) {
        for _, id := range userIDs {
            if !yield(id, nil) { return }
        }
    }
})

// From an iterator with known count
loader := probfilter.NewDataLoader(
    func() iter.Seq[string] { return slices.Values(userIDs) },
    probfilter.WithCount(int64(len(userIDs))),
)
```

### Trigger a rebuild

```go
rebuildable := filter.(*bloom.Filter) // or type-assert to RebuildableFilter
err := rebuildable.Rebuild(ctx, loader)
lastRebuild := rebuildable.LastRebuild()
```

## Collect statistics

Filters that implement `StatsProvider` expose runtime metrics.

```go
sp := filter.(probfilter.StatsProvider)
stats, err := sp.Stats(ctx)
```

| Field               | Type        | Description                                  |
|---------------------|-------------|----------------------------------------------|
| `Capacity`          | `int64`     | Maximum number of items the filter can hold  |
| `ItemCount`         | `int64`     | Approximate number of items currently stored |
| `FillRatio`         | `float64`   | Ratio of used capacity (0.0–1.0)             |
| `FalsePositiveRate` | `float64`   | Estimated current false-positive rate        |
| `MemoryUsageBytes`  | `int64`     | Memory used (zero for remote backends)       |
| `LastRebuild`       | `time.Time` | Time of last rebuild (Bloom only)            |
| `StorageType`       | `string`    | Backend identifier (`"memory"`, `"redis"`)   |

## Choose a storage backend

|                | Memory                                       | Redis                           |
|----------------|----------------------------------------------|---------------------------------|
| Latency        | Nanoseconds                                  | Network round-trip              |
| Persistence    | Process lifetime                             | Survives restarts               |
| Sharing        | Single process                               | Multiple processes              |
| Bloom options  | `WithExpectedItems`, `WithFalsePositiveRate` | Same + `WithKeyPrefix`          |
| Cuckoo options | `WithCapacity`                               | `WithCapacity`, `WithKeyPrefix` |
| Requirement    | None                                         | `redis.UniversalClient`         |

## API reference

| Interface           | Package              | Key methods                                                             |
|---------------------|----------------------|-------------------------------------------------------------------------|
| `Filter`            | `probfilter`         | `Add`, `AddBatch`, `MightExist`                                         |
| `DeletableFilter`   | `probfilter`         | `Filter` + `Delete`                                                     |
| `RebuildableFilter` | `probfilter`         | `Filter` + `Rebuild`, `LastRebuild`                                     |
| `StatsProvider`     | `probfilter`         | `Stats`                                                                 |
| `DataLoader`        | `probfilter`         | `StreamValues`, `Count`                                                 |
| `Manager`           | `probfilter`         | `Register`, `Get`, `MustGet`, `Unregister`, `Names`, `Filters`, `Close` |
| `FilterBuilder`     | `probfilter/factory` | `NewFilter` → `UseLogger`, `UseRedisClient`, `Build`                    |
| `ManagerBuilder`    | `probfilter/factory` | `NewManager` → `UseLogger`, `UseRedisClient`, `Build`                   |

## Handle errors

| Error                    | Package                  | Returned when                                   |
|--------------------------|--------------------------|-------------------------------------------------|
| `ErrFilterNotFound`      | `probfilter`             | `Manager.Get` called with an unregistered name  |
| `ErrFilterAlreadyExists` | `probfilter`             | `Manager.Register` called with a duplicate name |
| `ErrFilterFull`          | `cuckoo/storages/memory` | Cuckoo filter capacity exhausted                |
