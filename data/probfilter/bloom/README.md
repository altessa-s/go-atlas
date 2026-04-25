# bloom

```go
import "github.com/altessa-s/go-atlas/data/probfilter/bloom"
```

Package `bloom` provides a Bloom filter implementation for probabilistic existence checks. Bloom filters are space-efficient but do not support
deletion. Use periodic rebuilds via `RebuildableFilter` when the underlying dataset changes.

## Key types

| Type      | Description                                                                 |
|-----------|-----------------------------------------------------------------------------|
| `Filter`  | Bloom filter — implements `Filter`, `RebuildableFilter`, and `StatsProvider` |

## Options

| Option       | Default   | Description                |
|--------------|-----------|----------------------------|
| `WithLogger` | discard   | Sets the structured logger |

## Subpackages

| Package                        | Description                |
|--------------------------------|----------------------------|
| [storages](./storages)         | Storage interface          |
| [storages/memory](./storages/memory) | In-memory backend    |
| [storages/redis](./storages/redis)   | Redis-backed backend |
