# memory

```go
import "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/memory"
```

In-process, concurrency-safe [`retryqueue.Storage`](../../storage.go) backed by a map: the reference implementation of the retry queue contract, for
tests and single-process deployments. Items are lost on process exit, and resolved items are kept until deleted (there is no retention).

## Key types

| Symbol               | Description                                                         |
|----------------------|---------------------------------------------------------------------|
| `Store[P]`           | Map-backed `retryqueue.Storage[P]`; every method runs under a mutex |
| `New[P]() *Store[P]` | Constructs an empty store                                           |
| `NewWithClone[P](clone)` | Constructs a store with a custom payload snapshot function |
| `(*Store[P]).Len()`  | Number of stored items in any state                                 |
| `(*Store[P]).Move`   | `retryqueue.Mover`: rekeys an item under the mutex, fields kept     |

## Behavior

- Each transition is atomic, so `Mark` has one inserting winner and of racing `Expire` or `Flag` calls exactly one reports `true`.
- Every method checks its context before waiting for the mutex and again once it holds it: a call whose context is done, or ended while it
  waited, fails with the context error (`errors.Is(err, context.DeadlineExceeded)`, as from the MongoDB backend) and changes nothing.
- Instants are normalized to UTC. The store snapshots exported payload data on insert and read, including nested pointers, slices and map values.
  Private fields, channels and functions retain their identity. A pointer used in a map key retains its identity wherever it appears in the payload.
  Treat its target as immutable or use `NewWithClone` to snapshot both keys and references according to the payload's lookup rules.
- Worker methods implement `retryqueue.AttemptStorage` and check both the generation and the age observed by the caller.
- The store passes the [storagetest](../storagetest) conformance kit.

## Usage

```go
queue := memory.New[Valuation]()
item, err := queue.Mark(ctx, retryqueue.Item[Valuation]{Key: key, Since: now, NextAttemptAt: now})
```
