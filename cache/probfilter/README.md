# probfilter

```go
import "github.com/altessa-s/go-atlas/cache/probfilter"
```

Package `probfilter` provides probabilistic data structures for space-efficient existence checks. Supports Bloom and Cuckoo filters with pluggable
storage backends (memory, Redis). Includes a `Manager` for registering and retrieving named filter instances.

## Key types

| Type / Interface            | Description                                                                                                                              |
|-----------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `Filter`                    | Core interface: MightExist, Add, AddBatch                                                                                                |
| `DeletableFilter`           | Extends Filter with Delete (Cuckoo)                                                                                                      |
| `RebuildableFilter`         | Extends Filter with atomic Rebuild (Bloom, Cuckoo)                                                                                       |
| `ObservableFilter`          | Extends Filter with SetObserver (Bloom, Cuckoo)                                                                                          |
| `RebuildCommitReporter`     | Reports whether the contents come from a committed rebuild, also one committed by another process sharing a Redis filter (Bloom, Cuckoo) |
| `RebuildCommitTimeReporter` | Reports when the last rebuild committed, also one committed by another process sharing a Redis filter (Bloom, Cuckoo)                    |
| `RebuildLoadReporter`       | Reports how many values the last committed rebuild loaded from its source, also for a rebuild by another process (Bloom, Cuckoo)         |
| `Observer`                  | Receives lookup, add, and rebuild outcomes                                                                                               |
| `StatsProvider`             | Provides filter statistics                                                                                                               |
| `Manager`                   | Registry for named filter instances                                                                                                      |
| `DataLoader`                | Streams values and their count for `Rebuild`; `NewDataLoader` wraps an iterator (`WithCount` sets the count)                             |

## Manager options

| Option      | Default | Description                                                                    |
|-------------|---------|--------------------------------------------------------------------------------|
| `WithMeter` | nil     | Sets the metrics collector; enables metrics for registered `ObservableFilter`s |

With a collector, `Register` attaches an observer to every `ObservableFilter` and records `probfilter_lookups_total{filter_name,result}` (`result`:
`positive`, `negative`, `error`), `probfilter_lookup_duration_seconds{filter_name}`, `probfilter_adds_total{filter_name}`,
`probfilter_rebuild_duration_seconds`, and `probfilter_rebuild_errors_total`. A rebuild refused because another process rebuilds the shared filter
(`ErrRebuildInProgress`) is a skip and records neither; a superseded or otherwise failed rebuild counts as an error. `Unregister` and `Close` detach
the
observer. `Close` closes filters implementing `io.Closer` or `Close(context.Context) error`.

## Errors

| Error                     | Description                                                                               |
|---------------------------|-------------------------------------------------------------------------------------------|
| `ErrFilterNotFound`       | Returned when a filter name is not registered                                             |
| `ErrFilterAlreadyExists`  | Returned when a filter name is already in use                                             |
| `ErrFilterClosed`         | Returned by `Rebuild` on a closed filter                                                  |
| `ErrCommitIndeterminate`  | Wrapped by `Rebuild` when a commit's outcome is unknown; old or new contents are in place |
| `ErrRebuildInProgress`    | Wrapped by `Rebuild` when another process rebuilds the shared filter; nothing was loaded  |
| `ErrRebuildSuperseded`    | Wrapped by `Rebuild` when the rebuild lost its lease; its snapshot was discarded          |
| `ErrUnverifiableTopology` | Redis topology discovery could not establish the complete primary set |
| `ErrUnsafeEvictionPolicy` | Wrapped by the factory's Redis check when the server has an `allkeys-*` eviction policy   |

## Bloom vs Cuckoo

- **Bloom** — lower memory per item, supports periodic rebuilds (`RebuildableFilter`), no deletion.
- **Cuckoo** — supports individual deletion (`DeletableFilter`) and rebuilds, slightly higher memory overhead, can become full.

Rebuilds are atomic: lookups see the previous contents until the replacement takes over in one step, a failed or canceled rebuild leaves them
unchanged
(except an error wrapping `ErrCommitIndeterminate`, after which either the previous or the rebuilt contents are in place), and values added through
the filter during the rebuild are kept.

A Redis filter is shared by every process using its key. `RebuildCommitted` tells any of them whether some process committed a rebuild of it: the
commit
sets a ready marker after its rename, and every probfilter path that creates the filter key deletes the marker in the same step (live inserts use the
non-creating `BF.INSERT`/`CF.INSERT ... NOCREATE` form; a missing key is recreated through the reserve path). Delete the filter key together with its
`__probfilter__:` metadata keys; a key recreated by another client is not detected.

`LastRebuild` only knows the rebuilds of the calling process. `LastRebuildCommit` (`RebuildCommitTimeReporter`) returns the commit time of the last
rebuild by any process: the commit script stores the Redis server time as the value of the ready marker, in the same step as the promotion, so every
process reads the same instant whatever its own clock says. It costs one Redis round trip per call and is the zero time when `RebuildCommitted` is
false. A memory-backed filter returns its `LastRebuild`.

`RebuildCommitAge` (`RebuildCommitAgeReporter`) returns the age of that commit together with its time, both from the Redis clock in one script, so
a caller checking the age against a bound does not depend on how its own clock relates to the server's. A memory-backed filter uses the local clock.

`LastRebuildLoaded` (`RebuildLoadReporter`) returns how many values that rebuild loaded from its `DataLoader` — the size of the committed list. The
commit stores it in the same marker (`<unix ms>:<count>`), so values added to the filter afterwards, or replayed onto the rebuild, do not count, unlike
the item count of `Stats`. It is unknown (`known == false`) for a filter that was never rebuilt, a recreated key, and a marker written by a release that
stored only the time or `1`. A memory-backed filter reports its own last rebuild.

Choose Bloom when items are append-only or rebuilt in bulk. Choose Cuckoo when you need to remove individual items.

## Subpackages

| Package                                            | Description                  |
|----------------------------------------------------|------------------------------|
| [bloom](./bloom)                                   | Bloom filter implementation  |
| [bloom/storages/memory](./bloom/storages/memory)   | In-memory Bloom storage      |
| [bloom/storages/redis](./bloom/storages/redis)     | Redis-backed Bloom storage   |
| [cuckoo](./cuckoo)                                 | Cuckoo filter implementation |
| [cuckoo/storages/memory](./cuckoo/storages/memory) | In-memory Cuckoo storage     |
| [cuckoo/storages/redis](./cuckoo/storages/redis)   | Redis-backed Cuckoo storage  |
| [factory](../../factory/cache/probfilter)          | Configuration-based creation |
| [stats](./stats)                                   | Statistics definitions       |
