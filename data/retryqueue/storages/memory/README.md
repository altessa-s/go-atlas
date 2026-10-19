# memory

```go
import "github.com/altessa-s/go-atlas/data/retryqueue/storages/memory"
```

In-process, concurrency-safe [`retryqueue.Storage`](../../storage.go) backed by a map: the reference implementation of the retry queue contract, for
tests and single-process deployments. Items are lost on process exit, and resolved items are kept until deleted (there is no retention).

## Key types

| Symbol               | Description                                                         |
|----------------------|---------------------------------------------------------------------|
| `Store[P]`           | Map-backed `retryqueue.Storage[P]`; every method runs under a mutex |
| `New[P]() *Store[P]` | Constructs an empty store                                           |
| `(*Store[P]).Len()`  | Number of stored items in any state                                 |

## Behavior

- Each transition is atomic, so `Mark` has one inserting winner and of racing `Expire` or `Flag` calls exactly one reports `true`.
- Items are stored and returned by value and their instants normalized to UTC; a payload holding references (slices, maps, pointers) shares them with
  the caller.
- The store passes the [storagetest](../storagetest) conformance kit.

## Usage

```go
queue := memory.New[Valuation]()
item, err := queue.Mark(ctx, retryqueue.Item[Valuation]{Key: key, Since: now, NextAttemptAt: now})
```
