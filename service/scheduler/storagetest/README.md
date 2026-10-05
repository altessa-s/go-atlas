# storagetest

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storagetest"
```

Contract suite for `scheduler.Storage` implementations. Every bundled backend runs it, and a custom backend should too: the scheduler's correctness
rests on the atomic compare-and-swap writes, the run-ownership rules of the `Storage` godoc and the orderings checked here.

## Contracts

| Function                      | Verifies                                                                                                            |
|-------------------------------|---------------------------------------------------------------------------------------------------------------------|
| `Run`                         | Runs every contract below except `Identity`, `Pagination` and `History` as parallel subtests, each on a fresh store |
| `FinishRun`                   | Ownership-fenced completion; preserves pause/disable, a changed schedule and metadata; clears the lease; once       |
| `ReplaceTaskIf`               | Fenced replace on every `TaskFence` field including the revision; zero fields match absent ones                     |
| `CreateTask`                  | Insert-if-absent with revision 1; an existing (claimed) task is untouched; one of many concurrent creators wins     |
| `RenewRun`                    | Only the owner of an unfinished run renews its lease; the lease is bound to the run and cleared on finish           |
| `ClaimRunRequiresFinishedRun` | An active task whose previous run is unfinished cannot be claimed                                                   |
| `ClaimRunFencesOccurrence`    | A claim fences on `NextRunAt` and the one-shot `RunAt`, and stores the run's first lease                            |
| `RunIDRoundTrip`              | Run IDs with quotes, unicode or control characters survive claim, renewal and finish                                |
| `ClaimRunRejectsInvalidClaim` | An invalid `RunClaim` fails with `ErrInvalidRunClaim` and writes nothing                                            |
| `OwnedRunAnyNonZeroStart`     | Ownership requires `RunStartedAt != 0`, not a positive start                                                        |
| `ClaimRun`                    | Exactly one of many concurrent claims wins; stale occurrence fences and paused tasks lose                           |
| `DueTasks`                    | Exactly the active tasks with `NextRunAt <= now`, ordered by ID; `Tasks` returns every task (any order)             |
| `Identity`                    | IDs differing only by case or a trailing space are distinct; run ownership is case-sensitive; meta round-trips      |
| `Pagination`                  | Byte-wise ID order across pages; filters; history order `StartedAt DESC, ID DESC` with a compound cursor            |
| `History`                     | History order, retention cleanup, and `DeleteTask` removing a task's history                                        |
| `BenchmarkFinishRun`          | The conditional completion write with a fresh claim per iteration                                                   |

`Run` hands every contract its own store, which must be empty and isolated from the others. It leaves out `Identity`, `Pagination` and `History`:
the Redis backend does not satisfy them yet (history order, case-insensitive matching, prefix filters) and the MongoDB backend fails `Pagination`'s
`size()` filter, so only the memory and SQL backends run those three, on their own. Called on their own, `FinishRun` and `ReplaceTaskIf` tolerate a
store shared with other tests; the other contracts expect an empty, isolated store.

## Usage

```go
func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) scheduler.Storage { return newIsolatedStore(tb) })
	for _, check := range []func(*testing.T, scheduler.Storage){storagetest.Identity, storagetest.Pagination, storagetest.History} {
		check(t, newIsolatedStore(t))
	}
}
```

Live-database runs of the suite for the SQL backend live in [`tests/integration/schedulerit`](../../../tests/integration/schedulerit).
