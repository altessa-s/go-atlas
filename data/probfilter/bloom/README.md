# bloom

```go
import "github.com/altessa-s/go-atlas/data/probfilter/bloom"
```

Package `bloom` provides a Bloom filter implementation for probabilistic existence checks. Bloom filters are space-efficient but do not support
deletion. Use periodic rebuilds via `RebuildableFilter` when the underlying dataset changes.

`Rebuild` is atomic: the replacement is built off to the side (a fresh in-process filter, or a Redis staging key renamed onto the live key) while
lookups keep seeing the previous contents; a failed or canceled rebuild leaves them unchanged, and values added through the filter during the rebuild
are replayed onto the replacement. Writes by other processes to a shared Redis filter during the rebuild are not replayed.

## Key types

| Type      | Description                                                                 |
|-----------|-----------------------------------------------------------------------------|
| `Filter`  | Bloom filter — implements `Filter`, `RebuildableFilter`, `ObservableFilter`, and `StatsProvider` |

## Subpackages

| Package                        | Description                |
|--------------------------------|----------------------------|
| [storages](./storages)         | Storage interface          |
| [storages/memory](./storages/memory) | In-memory backend    |
| [storages/redis](./storages/redis)   | Redis-backed backend |
