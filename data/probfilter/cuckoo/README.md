# cuckoo

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
```

Package `cuckoo` provides a Cuckoo filter implementation for probabilistic existence checks. Unlike Bloom filters, Cuckoo filters support deletion of
individual items at the cost of slightly higher memory overhead.

`Rebuild` repopulates the filter from a `DataLoader` atomically, like the Bloom filter: lookups see the previous contents until the replacement (sized
for the loaded count plus 25% headroom, never below the configured capacity) replaces them; failure or cancellation leaves them unchanged (after
`probfilter.ErrCommitIndeterminate` either the previous or the rebuilt contents are in place); adds made through the same `Filter` during the rebuild
are replayed (adds by other processes sharing a Redis filter are lost on commit). Deletes made during a rebuild are **not** replayed — removing a
fingerprint from a differently populated filter could remove another member — so a deleted value still present in the source reappears as a possible
member (a false positive, never a false negative).

## Key types

| Type      | Description                                                              |
|-----------|--------------------------------------------------------------------------|
| `Filter`  | Cuckoo filter — implements `Filter`, `DeletableFilter`, `RebuildableFilter`, `ObservableFilter`, `RebuildCommitReporter`, and `StatsProvider` |

## Subpackages

| Package                        | Description                |
|--------------------------------|----------------------------|
| [storages](./storages)         | Storage interface          |
| [storages/memory](./storages/memory) | In-memory backend    |
| [storages/redis](./storages/redis)   | Redis-backed backend |
