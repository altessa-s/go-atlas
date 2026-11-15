# storages

```go
import "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
```

Package `storages` defines the `Storage` interface for Bloom filter backends. Implementations live in subpackages and are injected into the
`bloom.Filter` to supply the underlying bit array and hash operations.

## Key types

| Type / Interface            | Description                                                                                                                             |
|-----------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| `Storage`                   | Interface: MightExist, Add, AddBatch, Stage, LastRebuild, SetLastRebuild, Close                                                         |
| `Staging`                   | Replacement filter of a rebuild: AddBatch, Commit (atomic swap), Abort                                                                  |
| `ExclusiveRebuilder`        | Optional: shared storages serialize rebuilds with a lease (`BeginRebuild` → `RebuildLease`)                                             |
| `StatsProvider`             | Optional interface for backends that expose filter statistics                                                                           |
| `RebuildCommitReporter`     | Optional: shared (Redis) storages report whether a committed rebuild published the live filter                                          |
| `RebuildCommitTimeReporter` | Optional: shared (Redis) storages report the Redis time of the last rebuild committed by any process; `LastRebuild` stays process-local |
| `RebuildLoadReporter`       | Optional: shared (Redis) storages report how many values the last committed rebuild loaded from its source                              |

## Subpackages

| Package            | Description                |
|--------------------|----------------------------|
| [memory](./memory) | In-memory Bloom storage    |
| [redis](./redis)   | Redis-backed Bloom storage |
