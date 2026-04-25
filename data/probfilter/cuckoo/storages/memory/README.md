# memory

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
```

Package `memory` provides an in-memory storage backend for Cuckoo filters. Configurable capacity for the maximum number of items.
Thread-safe via `sync.RWMutex`.

## Options

| Option           | Default    | Description                              |
|------------------|------------|------------------------------------------|
| `WithCapacity`   | `100000`   | Maximum number of items the filter holds |

## Errors

| Error            | Description                                             |
|------------------|---------------------------------------------------------|
| `ErrFilterFull`  | Returned when the filter cannot accept more items        |
