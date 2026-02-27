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
