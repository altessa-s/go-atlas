# mongodb

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
```

Package `mongodb` implements `scheduler.Storage` using MongoDB as the backing store. Task states and execution history
are persisted in separate collections within the same database. Supports server-side CEL filter evaluation translated
to BSON queries for efficient paginated listing.

## Usage

```go
storage := mongodb.New(client.Database("mydb"))

// Create indexes once at startup (idempotent).
if err := storage.EnsureIndexes(ctx); err != nil {
    return err
}

sched := scheduler.New(storage)
```

## Options

| Option                  | Default              | Description                                                          |
|-------------------------|----------------------|----------------------------------------------------------------------|
| `WithTasksCollection`   | `scheduler_tasks`    | MongoDB collection name for persisting task state documents          |
| `WithHistoryCollection` | `scheduler_history`  | MongoDB collection name for persisting task execution history        |

## Indexes

Created by `EnsureIndexes`:

- **tasks**: unique `_id`, compound `(status, next_run)` for due-task queries, descending `priority` for dispatch order
- **history**: compound `(task_id, start_time desc)` for per-task history listing, TTL on `end_time` for auto-cleanup

## Filter support

CEL filter expressions are translated to BSON queries via the `mongotranslator` package and evaluated server-side by
MongoDB, avoiding full-collection scans for filtered paginated listing.
