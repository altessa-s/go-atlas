# memory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
```

Package `memory` provides an in-memory implementation of `scheduler.Storage`. All data lives in process memory and is
lost on exit, making this backend ideal for tests, local development, and single-instance deployments where durable
persistence is not required.

## Usage

```go
storage := memory.New(100) // keep up to 100 history entries per task
sched := scheduler.New(storage)
```

## Features

- Thread-safe: every method acquires an internal `sync.RWMutex`, safe for concurrent use from multiple goroutines
- Deep-copy semantics: returned values can be mutated freely without affecting the stored state
- Per-task history cap with oldest-first eviction — constructor argument sets the maximum entries per task
- In-memory CEL filter evaluation for `TasksPaginated` and `HistoryPaginated` via the `filter` AST visitor
- Binary search cursor seek for efficient cursor-based pagination without scanning the entire dataset
