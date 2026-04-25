# memory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
```

Package `memory` provides an in-memory implementation of `scheduler.Storage`. All data lives in process
memory and is lost on exit, making this backend ideal for tests, local development, and single-instance
deployments where durable persistence is not required.

## Features

- Thread-safe: every method acquires an internal `sync.RWMutex`, safe for concurrent use
- Deep-copy semantics: returned values can be mutated freely without affecting stored state
- Per-task history cap with oldest-first eviction -- constructor argument sets the maximum entries
- In-memory CEL filter evaluation for `TasksPaginated` and `HistoryPaginated` via the `filter` visitor
- Binary search cursor seek for efficient cursor-based pagination without scanning the entire dataset
