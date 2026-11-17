# retryqueue

```go
import "github.com/altessa-s/go-atlas/reliability/retryqueue"
```

Durable retry queue: one record of retryable work per key, swept by a periodic job until the work completes, expires or is dropped. A record carries a
generation, and every transition is fenced by it, so a worker holding a stale snapshot of the work never closes, reopens or deletes newer work.

## Key types

| Symbol             | Description                                                                                          |
|--------------------|------------------------------------------------------------------------------------------------------|
| `Item[P]`          | One record: key, generation, state, age (`Since`), attempts, next attempt, flag, caller payload `P`  |
| `State`            | `pending`, `expired` or `resolved`                                                                   |
| `Storage[P]`       | Persistence port implemented by the backends below                                                   |
| `Mover`            | Optional atomic `Move(ctx, from, to)` of an item to another key; every bundled backend implements it |
| `ErrItemNotFound`  | `Move` from a key without an item; wraps go-atlas-core `errors.ErrNotFound`                          |
| `ErrItemExists`    | `Move` onto a key that has an item; wraps go-atlas-core `errors.ErrAlreadyExists`                    |
| `DefaultListLimit` | 200, used when a list limit is not positive                                                          |

## Lifecycle

| Method           | Applies to                          | Effect                                                                                     |
|------------------|-------------------------------------|--------------------------------------------------------------------------------------------|
| `Mark`           | missing key                         | Inserts a fresh pending item; an existing item is returned unchanged (insert-only)         |
| `Reopen`         | older generation, or closed at same | Pending at the new generation with a new payload, age and schedule; attempts, flag cleared |
| `ListDue`        | pending, `NextAttemptAt ≤ now`      | Oldest next attempt first, ties by key                                                     |
| `WakeDue`        | pending, `NextAttemptAt > now`      | Due at `now`; no attempt counted                                                           |
| `Reschedule`     | pending at generation               | `Attempts++`, next attempt moved                                                           |
| `Expire`         | pending at generation and age       | `expired`, kept for operators; reports whether it expired                                  |
| `Resolve`        | pending or expired at generation    | `resolved`, expiry and flag dropped; kept until the storage's retention purges it          |
| `Flag`           | pending at generation               | Attention flag with a reason; reports whether the flag is new or its reason changed        |
| `ClearFlag`      | any state at generation             | Flag dropped                                                                               |
| `DeleteObsolete` | generation ≤ argument               | Deleted; reports whether a newer item exists instead                                       |
| `Delete`         | any                                 | Deleted                                                                                    |
| `ListByState`    | one state                           | Oldest `Since` first, ties by key                                                          |
| `Requeue`        | expired (all, or the given keys)    | Pending, due now, fresh age, no attempts; returns how many moved                           |

Instants come from the caller's clock. A transition on a missing item, or at another generation, is a no-op, not an error. An item written before
generations were used reads as generation 0.

## Moving an item

`Mover.Move(ctx, from, to)` renames the key of an item (the work it stands for was rekeyed) and keeps every other field as stored: generation,
state, age, attempts, schedule, expiry, resolution, flag and payload, so the item stays under the same fences. It is atomic — no other operation
sees the item under both keys or under neither — and fails with `ErrItemNotFound` when `from` has no item (checked first) and with `ErrItemExists`
when `to` has one, changing nothing; a move onto the item's own key changes nothing. `Mover` is separate from `Storage` so implementations outside
this module keep compiling: assert it (`queue.(retryqueue.Mover)`) or use a backend's concrete type.

```go
switch err := queue.Move(ctx, oldKey, newKey); {
case errors.Is(err, retryqueue.ErrItemExists):
    err = queue.Delete(ctx, oldKey) // the new key has its own item
case errors.Is(err, retryqueue.ErrItemNotFound):
    err = nil // nothing to move
}
```

## Concurrency

`ListDue` is a read, not a lease: two sweepers may attempt the same item. Correctness comes from the fenced transitions — `Mark` has exactly one
inserting winner, and of racing `Expire` or `Flag` calls exactly one reports `true` — so an operator alert or log line raised on `true` fires once.
`Expire` is fenced by the age the caller read, so an attempt that started before an operator's `Requeue` cannot expire the requeued item.

## Backends

| Package                                          | Description                                                                         |
|--------------------------------------------------|-------------------------------------------------------------------------------------|
| [storages/memory](./storages/memory)             | In-process reference implementation                                                 |
| [storages/mongo](./storages/mongo)               | One document per item, payload inlined, partial due index, optional TTL             |
| [storages/sqldb](./storages/sqldb)               | PostgreSQL, MySQL and MariaDB: one row per item, `PurgeExpired(ctx) (int64, error)` |
| [storages/storagetest](./storages/storagetest)   | Conformance kit every backend passes                                                |

[`factory/reliability/retryqueue`](../../factory/reliability/retryqueue) builds a store from [`config/retryqueue`](../../config/retryqueue)
(`storage.type: memory | mongo | sqldb`); the mongo store registers from its `storages/mongo` subpackage.

## Usage

```go
queue, err := retryqueuemongo.New[Valuation](db.Collection("valuations"), retryqueuemongo.WithResolvedRetention(7*24*time.Hour))
if err != nil {
    return err
}
if err := queue.EnsureIndexes(ctx); err != nil {
    return err
}

due, err := queue.ListDue(ctx, now, 0)
for _, item := range due {
    switch err := attempt(ctx, item); {
    case err == nil:
        _ = queue.Resolve(ctx, item.Key, item.Generation, now)
    case now.Sub(item.Since) >= maxAge:
        _, _ = queue.Expire(ctx, item.Key, item.Generation, item.Since, now)
    default:
        _ = queue.Reschedule(ctx, item.Key, item.Generation, now.Add(retryInterval))
    }
}
```
