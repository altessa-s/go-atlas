# storagetest

```go
import "github.com/altessa-s/go-atlas/data/retryqueue/storages/storagetest"
```

Conformance kit for [`retryqueue.Storage`](../../storage.go) backends. `Run` executes every contract as a parallel subtest on a fresh store from the
caller's constructor, which must return an empty store isolated from every other store it returns.

## Key types

| Symbol                      | Description                                                              |
|-----------------------------|--------------------------------------------------------------------------|
| `Run(t, newStore, skip...)` | Runs all contracts; the names in `skip` are reported as skipped          |
| `Payload`                   | The caller data the contracts store (`valueOn`, `note`, `tag` omitempty) |
| `Storage`, `Item`           | `retryqueue.Storage[Payload]` and `retryqueue.Item[Payload]`             |
| `T0`, `Pending`             | The reference instant and a generation-1 pending item fixture            |

## Contracts

| Group     | Contracts                                                                                                                                                 |
|-----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| Marking   | `MarkIsInsertOnly`, `MarkStoresFreshItem`, `ConcurrentMark`                                                                                               |
| Due list  | `ListDueOrderAndReschedule`, `ListDueLimit`, `WakeDueMakesPendingDue`                                                                                     |
| Closing   | `ExpireKeepsItemButStopsListing`, `ResolveKeepsPinnedPayload`, `ResolveIsIdempotentAndClearsExpiry`, `DeleteIsIdempotent`, `ConcurrentExpire`             |
| Flags     | `FlagOnce`, `FlagReasonChangeAndClear`, `FlagOnlyPending`, `ConcurrentFlag`                                                                               |
| Operators | `RequeueRevivesExpired`, `StaleExpireSparesRequeued`, `ListByStateOrder`                                                                                  |
| Fencing   | `ReopenMovesToNewerGenerationOnly`, `ReopenSameGeneration`, `ReopenMissingInserts`, `ReopenReplacesPayload`, `StaleGenerationIsNoOp`, `MissingItemIsNoOp` |

## Usage

```go
func TestStorageContract(t *testing.T) {
    t.Parallel()
    storagetest.Run(t, func(testing.TB) storagetest.Storage { return memory.New[storagetest.Payload]() })
}
```
