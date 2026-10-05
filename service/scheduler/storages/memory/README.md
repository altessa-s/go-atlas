# memory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
```

Package `memory` provides an in-memory implementation of `scheduler.Storage`. All data lives in process memory and is lost on exit, making this backend
ideal for tests, local development, and single-instance deployments where durable persistence is not required.

## Features

- Thread-safe: every method acquires an internal `sync.RWMutex`, safe for concurrent use
- Deep-copy semantics: returned values can be mutated freely without affecting stored state
- Per-task history cap with oldest-first eviction -- constructor argument sets the maximum entries
- In-memory CEL filter evaluation for `TasksPaginated` and `HistoryPaginated` via the `filter` visitor
- Binary search cursor seek for efficient cursor-based pagination without scanning the entire dataset

## Atomic run operations

`ClaimRun`, `CreateTask`, `RenewRun` and `FinishRun` each check and write under the storage mutex, so every one of them is atomic.

## Atomic run finalization

`FinishRun` uses a mutex-protected update to compare `last_run_id` and the unfinished-run marker before updating execution fields. It rejects stale or
repeated completion, preserves current task configuration and paused/disabled status, and does not advance a concurrently changed schedule. Claiming and
finishing protect scheduler state; external task side effects still need idempotency when abandoned runs can be retried.
