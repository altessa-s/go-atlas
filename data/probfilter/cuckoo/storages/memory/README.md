# memory

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
```

Package `memory` provides an in-memory storage backend for Cuckoo filters (4-slot buckets, 8-bit fingerprints). Configurable capacity for the maximum
number of items. Thread-safe via `sync.RWMutex`.

A failed insert (`ErrFilterFull`) undoes its relocation walk and leaves the filter unchanged, so it never evicts a previously added value. Deleting a
value that was never added can still remove a colliding member's fingerprint — only delete values that were added.

## Options

| Option           | Default    | Description                              |
|------------------|------------|------------------------------------------|
| `WithCapacity`   | `100000`   | Maximum number of items the filter holds |

## Errors

| Error            | Description                                             |
|------------------|---------------------------------------------------------|
| `ErrFilterFull`  | Returned when the filter cannot accept more items; the filter is unchanged |
