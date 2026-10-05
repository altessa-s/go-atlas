# mongodb

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/mongodb"
```

Package `mongodb` implements `scheduler.Storage` using MongoDB as the backing store. Task states and execution history are persisted in separate
collections within the same database. Supports server-side CEL filter evaluation translated to BSON queries for efficient paginated listing.

## Options

| Option                  | Default              | Description                                                 |
|-------------------------|----------------------|-------------------------------------------------------------|
| `WithTasksCollection`   | `scheduler_tasks`    | MongoDB collection name for persisting task state documents |
| `WithHistoryCollection` | `scheduler_history`  | MongoDB collection name for persisting execution history    |

## Indexes

Created by `EnsureIndexes`:

- **tasks**: unique `_id`, compound `(status, next_run_at)` for due-task queries, descending `priority`
- **history**: compound `(task_id, started_at desc, _id desc)` matching the sort of both `History` and `HistoryPaginated`, ascending `ended_at` for
  `CleanupHistory`

Every index key must name a field that `taskDocument` / `historyDocument` actually persists — MongoDB accepts an index on an absent field and then never
uses it. `TestIndexKeys` pins the two together.

History retention runs through `CleanupHistory`, not a TTL index: `EndedAt` is a Unix timestamp stored as `int64`, and MongoDB TTL indexes only expire
documents whose indexed field holds a BSON date.

Deployments created before this was corrected still carry inert indexes on the nonexistent fields `next_run`, `start_time`, and `end_time`; drop them
manually.

## Atomic run claim

`ClaimRun` transitions a task `active → running` for a specific occurrence with a single conditional `UpdateOne` (matched on `_id`, `status`, a zero or
absent `run_started_at`, and — when fenced — `next_run_at` and `run_at`). MongoDB applies the document update atomically, so among concurrent
schedulers exactly one match succeeds and exactly one claims the run; the rest see `ModifiedCount == 0` and skip. This prevents concurrent claims of the
same active occurrence. The rules themselves are defined once, in the godoc of `scheduler.Storage` ("Run ownership").

`CreateTask` is an `InsertOne` whose duplicate-key error on `_id` reports an existing task, and `RenewRun` is a conditional `UpdateOne` on `_id`,
`last_run_id` and a positive `run_started_at` that sets `run_lease_until` and increments `revision`.

## Filter support

CEL filter expressions are translated to BSON queries via the `mongotranslator` package and evaluated server-side by MongoDB, avoiding full-collection
scans for filtered paginated listing.

Documents omit zero-valued fields (`omitempty`: `description`, `lastRunAt`, `nextRunAt`, `skipNextRun`, `disableHistory`, `unmanaged`,
`oneShot`; history `error`). The translator is told so through `filter.WithZeroWhenAbsent` — derived from the document structs' tags — and
matches an absent field exactly as its zero value: `description == ""`, `oneShot == false`, `nextRunAt == 0` and `description.size() == 0`
select tasks stored without those fields, `description != ""` does not, and documents that store the zeros explicitly behave the same. No data
migration is needed. This assumes the collections use MongoDB's default simple collation, as the ones this storage creates do; a collection
created with a locale collation may compare a stored `""` differently from how the translator judges an absent `description`.

## Atomic run finalization

`FinishRun` uses a conditional update pipeline to compare `last_run_id` and the unfinished-run marker before updating execution fields. It rejects stale
or repeated completion, preserves current task configuration and paused/disabled status, and does not advance a concurrently changed schedule. Claiming
and finishing protect scheduler state; external task side effects still need idempotency when abandoned runs can be retried.
