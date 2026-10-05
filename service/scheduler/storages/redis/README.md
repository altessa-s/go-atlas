# redis

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
```

Package `redis` implements `scheduler.Storage` using RedisJSON for document storage and RediSearch for indexed querying. Safe for concurrent use across
multiple processes, so several service instances can share scheduler state through a single Redis cluster.

## Prerequisites

The Redis server must have the **RedisJSON** and **RediSearch** modules loaded.

## Options

| Option                  | Default       | Description                                                     |
|-------------------------|---------------|-----------------------------------------------------------------|
| `WithKeyPrefix`         | `scheduler`   | Prefix for all Redis keys, used to namespace multiple schedulers|
| `WithHistoryTTL`        | 0 (no expiry) | TTL applied to history entry keys for automatic expiration      |
| `WithMaxHistoryPerTask` | 0 (unlimited) | Auto-trim oldest entries per task when the cap is exceeded      |

## Zero next run

`next_run_at` is stored even when zero: RediSearch does not index a missing NUMERIC field, so a document without it never matches the `DueTasks`
range `@nextRunAt:[-inf now]`, and an active task with a zero `NextRunAt` — due on every other backend — would never run. Documents written before
(when the field was omitted at zero) are migrated by `EnsureIndexes`: once the task index has finished indexing, it sets `next_run_at` to `0` on every
task document that lacks it (or holds null), in a Lua script per key, without changing the revision (absent and zero are the same state to every
fence). It re-reads the first page of remaining matches after each batch instead of paging by offset, so documents that other instances rewrite
meanwhile cannot make it skip the rest. The backfill is idempotent and costs one search per call once nothing is left to migrate. During a rolling upgrade, instances still running an older
version keep writing the field-less form; run `EnsureIndexes` again (any restart does) after the rollout to migrate those documents too.

## Atomic run claim

`ClaimRun` transitions a task `active → running` for a specific occurrence with a single server-side Lua `EVAL` script (read status, `run_started_at`,
`next_run_at` and `run_at`, check the fence, then `JSON.SET` the new state). Redis runs the script atomically under its single-threaded execution, so
among concurrent schedulers exactly one claim returns `1` and wins the run; the rest get `0` and skip. This prevents concurrent claims of the same
active occurrence. The rules themselves are defined once, in the godoc of `scheduler.Storage` ("Run ownership").

`CreateTask` (store only when the key does not exist) and `RenewRun` (set `run_lease_until` and `run_lease_id` while `last_run_id` and an unfinished
`run_started_at` still match) are Lua scripts as well, so each check and write is atomic. Every script runs through `redis.Script`: `EVALSHA` by digest,
falling back to `EVAL` when the server answers `NOSCRIPT` (after a restart or `SCRIPT FLUSH`), so the script body is not sent on every call.

## Filter support

CEL filter expressions are evaluated on the client with the same `data/filter` evaluator as the memory backend, so a filter selects the same tasks
on every backend. RediSearch cannot evaluate CEL exactly — TAG fields fold case, TEXT fields are tokenized, zero values left out of a document are
absent from the index, and `endsWith`, `matches` and `size()` have no query form — so only the part it evaluates identically is pushed down through
the `redisearch` translator to narrow the scan: comparisons and non-empty `in` lists over `status`, `priority` and `failures` (history: `startedAt`,
`endedAt`, `durationMs`), joined by `&&`, `||` and `!`. `HistoryPaginated` also pushes its cursor down as a `startedAt` range and orders ties by
ID on the client, since RediSearch sorts by one field only.

History lookups (`History`, `HistoryPaginated`, `DeleteTask`, the per-task trim) query the `taskId` TAG, which folds case, and then compare each
entry's task ID exactly, so tasks whose IDs differ only by case never see or remove each other's history. Once a task's candidate count passes the
cap, the trim walks its candidates' task IDs instead of only the overflow keys.

## Atomic run finalization

`FinishRun` uses a server-side Lua script to compare `last_run_id` and the unfinished-run marker before updating execution fields. It rejects stale or
repeated completion, preserves current task configuration and paused/disabled status, and does not advance a concurrently changed schedule. Claiming and
finishing protect scheduler state; external task side effects still need idempotency when abandoned runs can be retried.
