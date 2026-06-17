# mongo

```go
import sagamongo "github.com/altessa-s/go-atlas/data/saga/storages/mongo"
```

Durable [`saga.Store`](../../store.go) backed by a MongoDB collection. Each saga instance is one document keyed by its ID; the document's `version`
field is the optimistic-concurrency token, so `Update` is a version-checked write that rejects a stale writer with `errs.ErrVersionConflict` —
concurrent recovery cycles cannot double-advance the same instance.

## Constructors

| Function                          | Description                                                                      |
|-----------------------------------|----------------------------------------------------------------------------------|
| `New(db, opts...)`                | Store on `db.Collection(name)`; creates indexes.                                 |
| `NewWithCollection(col, opts...)` | Store on an existing collection handle; `WithCollectionName` is ignored.         |

## Options

| Option                    | Default                | Description                                       |
|---------------------------|------------------------|---------------------------------------------------|
| `WithCollectionName`      | `saga_instances`       | Collection name (ignored by `NewWithCollection`). |
| `WithContext`             | `context.Background()` | Context used for index creation.                  |
| `WithIndexCreateTimeout`  | 10s                    | Timeout applied to initial index creation.        |

## Indexes

`New` creates a compound index on `(status, deadline)` that backs `FetchRecoverable`: the recovery scan filters by `status`, and by `deadline` for the
timed-out `RUNNING` branch. The `status` prefix also serves the status-only `COMPENSATING` branch. Index creation is idempotent.

## Document shape

Instance-level timestamps are stored as Unix seconds (`0` for an unset deadline), so `FetchRecoverable` evaluates deadline eligibility at one-second
granularity. The instance ID is the document `_id`; `data` is the opaque serialized saga payload.

| BSON field   | Source                |
|--------------|-----------------------|
| `_id`        | `Instance.ID`         |
| `definition` | `Instance.Definition` |
| `status`     | `Instance.Status`     |
| `stage`      | `Instance.Stage`      |
| `data`       | `Instance.Data`       |
| `steps`      | `Instance.Steps`      |
| `created_at` | `Instance.CreatedAt`  |
| `updated_at` | `Instance.UpdatedAt`  |
| `deadline`   | `Instance.Deadline`   |
| `version`    | `Instance.Version`    |
| `last_error` | `Instance.LastError`  |

## See also

- [storages/memory](../memory) — in-process reference backend.
- [storages/nats](../nats) — durable NATS JetStream KeyValue backend.
