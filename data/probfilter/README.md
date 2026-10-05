# probfilter

```go
import "github.com/altessa-s/go-atlas/data/probfilter"
```

Package `probfilter` provides probabilistic data structures for space-efficient existence checks. Supports Bloom and Cuckoo filters with
pluggable storage backends (memory, Redis). Includes a `Manager` for registering and retrieving named filter instances.

## Key types

| Type / Interface    | Description                                        |
|---------------------|----------------------------------------------------|
| `Filter`            | Core interface: MightExist, Add, AddBatch          |
| `DeletableFilter`   | Extends Filter with Delete (Cuckoo)                |
| `RebuildableFilter` | Extends Filter with atomic Rebuild (Bloom, Cuckoo) |
| `ObservableFilter`  | Extends Filter with SetObserver (Bloom, Cuckoo)    |
| `Observer`          | Receives lookup, add, and rebuild outcomes         |
| `StatsProvider`     | Provides filter statistics                         |
| `Manager`           | Registry for named filter instances                |

## Manager options

| Option          | Default | Description                                                                    |
|-----------------|---------|--------------------------------------------------------------------------------|
| `WithCollector` | nil     | Sets the metrics collector; enables metrics for registered `ObservableFilter`s |

With a collector, `Register` attaches an observer to every `ObservableFilter` and records `probfilter_lookups_total{filter_name,result}` (`result`:
`positive`, `negative`, `error`), `probfilter_lookup_duration_seconds{filter_name}`, `probfilter_adds_total{filter_name}`,
`probfilter_rebuild_duration_seconds`, and `probfilter_rebuild_errors_total`. `Unregister` and `Close` detach it. `Close` closes filters implementing
`io.Closer` or `Close(context.Context) error`.

## Errors

| Error                    | Description                                                                               |
|--------------------------|-------------------------------------------------------------------------------------------|
| `ErrFilterNotFound`      | Returned when a filter name is not registered                                             |
| `ErrFilterAlreadyExists` | Returned when a filter name is already in use                                             |
| `ErrFilterClosed`        | Returned by `Rebuild` on a closed filter                                                  |
| `ErrCommitIndeterminate` | Wrapped by `Rebuild` when a commit's outcome is unknown; old or new contents are in place |
| `ErrRebuildInProgress`   | Wrapped by `Rebuild` when another process rebuilds the shared filter; nothing was loaded  |
| `ErrRebuildSuperseded`   | Wrapped by `Rebuild` when the rebuild lost its lease; its snapshot was discarded          |

## Bloom vs Cuckoo

- **Bloom** — lower memory per item, supports periodic rebuilds (`RebuildableFilter`), no deletion.
- **Cuckoo** — supports individual deletion (`DeletableFilter`) and rebuilds, slightly higher memory overhead, can become full.

Rebuilds are atomic: lookups see the previous contents until the replacement takes over in one step, a failed or canceled rebuild leaves them unchanged,
and values added through the filter during the rebuild are kept.

Choose Bloom when items are append-only or rebuilt in bulk. Choose Cuckoo when you need to remove individual items.

## Subpackages

| Package                                          | Description                    |
|--------------------------------------------------|--------------------------------|
| [bloom](./bloom)                                 | Bloom filter implementation    |
| [bloom/storages/memory](./bloom/storages/memory) | In-memory Bloom storage        |
| [bloom/storages/redis](./bloom/storages/redis)   | Redis-backed Bloom storage     |
| [cuckoo](./cuckoo)                               | Cuckoo filter implementation   |
| [cuckoo/storages/memory](./cuckoo/storages/memory) | In-memory Cuckoo storage    |
| [cuckoo/storages/redis](./cuckoo/storages/redis) | Redis-backed Cuckoo storage    |
| [factory](./factory)                             | Configuration-based creation   |
| [stats](./stats)                                 | Statistics definitions         |
