# mongodb

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
```

Package `mongodb` implements `scheduler.Storage` using MongoDB as the backing store. Task states and
execution history are persisted in separate collections within the same database. Supports server-side
CEL filter evaluation translated to BSON queries for efficient paginated listing.

## Options

| Option                  | Default              | Description                                                 |
|-------------------------|----------------------|-------------------------------------------------------------|
| `WithTasksCollection`   | `scheduler_tasks`    | MongoDB collection name for persisting task state documents |
| `WithHistoryCollection` | `scheduler_history`  | MongoDB collection name for persisting execution history    |

## Indexes

Created by `EnsureIndexes`:

- **tasks**: unique `_id`, compound `(status, next_run)` for due-task queries, descending `priority`
- **history**: compound `(task_id, start_time desc)` for per-task listing, TTL on `end_time`

## Atomic run claim

`ClaimRun` transitions a task `active → running` for a specific occurrence with a single conditional `UpdateOne` (matched on `_id`, `status`, and — when
fenced — `next_run_at`). MongoDB applies the document update atomically, so among concurrent schedulers exactly one match succeeds and exactly one claims
the run; the rest see `ModifiedCount == 0` and skip. This makes duplicate execution impossible even during a leader-election split-brain.

## Filter support

CEL filter expressions are translated to BSON queries via the `mongotranslator` package and evaluated
server-side by MongoDB, avoiding full-collection scans for filtered paginated listing.
