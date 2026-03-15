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
| `RebuildableFilter` | Extends Filter with Rebuild (Bloom)                |
| `StatsProvider`     | Provides filter statistics                         |
| `Manager`           | Registry for named filter instances                |

## Manager options

| Option           | Default   | Description                  |
|------------------|-----------|------------------------------|
| `WithLogger`     | discard   | Sets the structured logger   |
| `WithCollector`  | nil       | Sets the metrics collector   |

## Errors

| Error                     | Description                                   |
|---------------------------|-----------------------------------------------|
| `ErrFilterNotFound`       | Returned when a filter name is not registered  |
| `ErrFilterAlreadyExists`  | Returned when a filter name is already in use  |

## Bloom vs Cuckoo

- **Bloom** — lower memory per item, supports periodic rebuilds (`RebuildableFilter`), no deletion.
- **Cuckoo** — supports individual deletion (`DeletableFilter`), slightly higher memory overhead, can become full.

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
