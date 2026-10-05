# storages

```go
import "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages"
```

Package `storages` defines the `Storage` interface for Bloom filter backends. Implementations live in subpackages and are injected into the
`bloom.Filter` to supply the underlying bit array and hash operations.

## Key types

| Type / Interface     | Description                                                                                 |
|----------------------|---------------------------------------------------------------------------------------------|
| `Storage`            | Interface: MightExist, Add, AddBatch, Stage, LastRebuild, Close                             |
| `Staging`            | Replacement filter of a rebuild: AddBatch, Commit (atomic swap), Abort                      |
| `ExclusiveRebuilder` | Optional: shared storages serialize rebuilds with a lease (`BeginRebuild` → `RebuildLease`) |
| `StatsProvider`      | Optional interface for backends that expose filter statistics                               |

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [memory](./memory)     | In-memory Bloom storage            |
| [redis](./redis)       | Redis-backed Bloom storage         |
