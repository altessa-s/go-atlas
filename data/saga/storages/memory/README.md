# memory

```go
import "github.com/altessa-s/go-atlas/data/saga/storages/memory"
```

In-process, concurrency-safe [`saga.Store`](../../store.go) backed by a map. It is the reference implementation of the saga storage contract, suited to
single-node deployments and tests. State is lost on process exit, so it provides no cross-restart crash recovery — use a durable backend for that.

## Key types

| Symbol            | Description                                                         |
|-------------------|-------------------------------------------------------------------|
| `Store`           | Map-backed `saga.Store` with `sync.RWMutex` and version CAS.       |
| `New() *Store`    | Constructs an empty store.                                         |
| `(*Store).Len()`  | Number of stored instances (for tests and metrics).               |

## Behavior

| Method             | Notes                                                                          |
|--------------------|--------------------------------------------------------------------------------|
| `Create`           | Returns `errs.ErrInstanceExists` on a duplicate ID.                            |
| `Get`              | Returns a deep copy, or `errs.ErrInstanceNotFound`.                            |
| `Update`           | Compare-and-swap on `Version`; returns `errs.ErrVersionConflict` on a stale write and writes the new version back into the argument. |
| `FetchRecoverable` | Returns non-terminal instances that are mid-compensation or past their deadline. |
| `Delete`           | Idempotent; deleting a missing instance is not an error.                       |

## Usage

```go
store := memory.New()
orch := saga.New(store, def)
inst, err := orch.Start(ctx, id, data)
```
