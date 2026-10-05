# storagetest

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storagetest"
```

Contract suite for `scheduler.Storage` implementations. Every bundled backend runs it, and a custom backend should too: the scheduler's correctness
rests on the atomic compare-and-swap writes and the orderings checked here.

## Contracts

| Function             | Verifies                                                                                                           |
|----------------------|--------------------------------------------------------------------------------------------------------------------|
| `FinishRun`          | Ownership-fenced completion; preserves pause/disable, a changed schedule and metadata; a run finishes only once    |
| `ReplaceTaskIf`      | Fenced replace on every `TaskFence` field including the revision; zero fields match absent ones; revisions advance |
| `ClaimRun`           | Exactly one of many concurrent claims wins; stale occurrence fences and paused tasks lose                          |
| `DueTasks`           | Exactly the active tasks with `NextRunAt <= now`, ordered by ID; `Tasks` returns every task (any order)            |
| `Identity`           | IDs differing only by case or a trailing space are distinct; run ownership is case-sensitive; meta round-trips     |
| `Pagination`         | Byte-wise ID order across pages; filters; history order `StartedAt DESC, ID DESC` with a compound cursor           |
| `History`            | History order, retention cleanup, and `DeleteTask` removing a task's history                                       |
| `BenchmarkFinishRun` | The conditional completion write with a fresh claim per iteration                                                  |

`FinishRun` and `ReplaceTaskIf` tolerate a store shared with other tests; the other contracts expect an empty, isolated store.

## Usage

```go
func TestStorageContract(t *testing.T) {
	for name, check := range map[string]func(*testing.T, scheduler.Storage){
		"claim_run":  storagetest.ClaimRun,
		"pagination": storagetest.Pagination,
		// ...
	} {
		t.Run(name, func(t *testing.T) { check(t, newIsolatedStore(t)) })
	}
}
```

Live-database runs of the suite for the SQL backend live in [`tests/integration/schedulerit`](../../../tests/integration/schedulerit).
