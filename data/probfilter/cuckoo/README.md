# cuckoo

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
```

Package `cuckoo` provides a Cuckoo filter implementation for probabilistic existence checks. Unlike Bloom filters, Cuckoo filters support
deletion of individual items at the cost of slightly higher memory overhead.

## Key types

| Type      | Description                                                              |
|-----------|--------------------------------------------------------------------------|
| `Filter`  | Cuckoo filter — implements `Filter`, `DeletableFilter`, and `StatsProvider` |

## Subpackages

| Package                        | Description                |
|--------------------------------|----------------------------|
| [storages](./storages)         | Storage interface          |
| [storages/memory](./storages/memory) | In-memory backend    |
| [storages/redis](./storages/redis)   | Redis-backed backend |
