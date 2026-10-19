# mongo

```go
import retrymongo "github.com/altessa-s/go-atlas/data/retryqueue/storages/mongo"
```

MongoDB-backed [`retryqueue.Storage`](../../storage.go): one document per item in a caller-chosen collection, with the key as `_id` and the payload
`P` inlined next to the store's fields. Every transition is a single conditional write; `Mark` is an insert-only upsert repeated once on a
duplicate-key race (not inside a transaction, which the error has aborted). Integration tests run when `MONGO_URI` is set (see
[mongotest](../../../mongo/mongotest)).

## Key types

| Symbol                      | Description                                                                              |
|-----------------------------|------------------------------------------------------------------------------------------|
| `Store[P]`                  | The storage over a `*mongo.Collection`                                                   |
| `New[P](coll, opts...)`     | Constructs a store; `ErrInvalidFlagField` or `ErrInvalidPayload` on a field-name problem |
| `(*Store[P]).EnsureIndexes` | Creates the indexes below; idempotent                                                    |
| `Index*`                    | Index names: `idx_pending_next_attempt`, `idx_state_since`, `ttl_resolved`               |

## Options

| Option                  | Default      | Description                                                                       |
|-------------------------|--------------|-----------------------------------------------------------------------------------|
| `WithFlagField`         | `flagged`    | Document field of `Item.Flagged`                                                  |
| `WithFlagReasonField`   | `flagReason` | Document field of `Item.FlagReason`                                               |
| `WithResolvedRetention` | off          | TTL of resolved items (whole seconds, at least 1 s); off keeps them until deleted |

## Document

| Field                     | Type         | Notes                                                                    |
|---------------------------|--------------|--------------------------------------------------------------------------|
| `_id`                     | string       | `Item.Key`                                                               |
| `since`, `nextAttemptAt`  | date         | Caller instants, stored in UTC (millisecond precision)                   |
| `attempts`                | int          | Failed attempts                                                          |
| `state`                   | string       | `pending`, `expired`, `resolved`                                         |
| `expiredAt`, `resolvedAt` | date         | Absent unless set: a stored zero `resolvedAt` would be purged by the TTL |
| `generation`              | int64        | Absent in documents written before generations: reads as 0               |
| flag fields               | bool, string | Absent unless flagged                                                    |
| payload fields            | caller's     | Inlined; `New` rejects a payload field named like any field above        |

`P` must be a struct or a struct pointer without an inlined map: its document fields are then known statically, `New` checks them against the
store's, and decoding a document into `P` ignores the store's fields. `Reopen` replaces the matched document with a fresh one in one write
(`findAndModify` with a replacement), so the whole payload is replaced and no field of the previous generation survives.

## Query audit

| Query                                              | Filter / sort                                   | Index                      |
|----------------------------------------------------|-------------------------------------------------|----------------------------|
| `Mark`, `Reopen`, `DeleteObsolete`, `Reschedule`   | `_id` (+ `state` / `generation`)                | `_id_`                     |
| `Expire`, `Resolve`, `Flag`, `ClearFlag`, `Delete` | `_id` (+ `state` / `generation` / `since`)      | `_id_`                     |
| `ListDue`                                          | `{state: pending, nextAttemptAt ≤ now}`, sorted | `idx_pending_next_attempt` |
| `WakeDue`                                          | `{state: pending, nextAttemptAt > now}`         | `idx_pending_next_attempt` |
| `ListByState`                                      | `{state}` sort `since, _id`                     | `idx_state_since`          |
| `Requeue`                                          | `{state: expired[, _id ∈ keys]}`                | `idx_state_since` / `_id_` |
| TTL purge (with retention)                         | `resolvedAt` (resolved only)                    | `ttl_resolved`             |

Changing the retention of an existing TTL index makes `EnsureIndexes` fail with an index-options conflict: drop `ttl_resolved` first.

## Usage

```go
queue, err := retrymongo.New[Valuation](db.Collection("valuations"),
    retrymongo.WithFlagField("needsManualRate"), retrymongo.WithFlagReasonField("manualRateReason"),
    retrymongo.WithResolvedRetention(7*24*time.Hour))
if err != nil {
    return err
}
if err := queue.EnsureIndexes(ctx); err != nil {
    return err
}
```
