# storages

```go
import "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages"
```

Package `storages` defines the `Storage` interface for Cuckoo filter backends. Implementations live in subpackages and are injected into the
`cuckoo.Filter` to supply the underlying bucket array and fingerprint operations.

## Key types

| Type / Interface            | Description                                                                                                |
|-----------------------------|------------------------------------------------------------------------------------------------------------|
| `Storage`                   | Interface: MightExist, Add, AddBatch, Delete, Stage, Close                                                 |
| `Staging`                   | Replacement filter of a rebuild: AddBatch, Commit (atomic swap), Abort                                     |
| `ExclusiveRebuilder`        | Optional: shared storages serialize rebuilds with a lease (`BeginRebuild` → `RebuildLease`)                |
| `StatsProvider`             | Optional interface for backends that expose filter statistics                                              |
| `RebuildCommitReporter`     | Optional: shared (Redis) storages report whether a committed rebuild published the live filter             |
| `RebuildCommitTimeReporter` | Optional: shared (Redis) storages report the Redis time of the last rebuild committed by any process       |
| `RebuildLoadReporter`       | Optional: shared (Redis) storages report how many values the last committed rebuild loaded from its source |

## Subpackages

| Package            | Description                 |
|--------------------|-----------------------------|
| [memory](./memory) | In-memory Cuckoo storage    |
| [redis](./redis)   | Redis-backed Cuckoo storage |
