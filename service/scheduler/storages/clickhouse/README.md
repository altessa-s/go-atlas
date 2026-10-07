# clickhouse

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/clickhouse"
```

Package `clickhouse` keeps the scheduler's execution history in a ClickHouse table. It implements `scheduler.HistoryStorage`, not
`scheduler.Storage`: task state needs atomic compare-and-swap writes that ClickHouse does not offer, so it stays in another backend
([`memory`](../memory), [`mongo`](../mongo), [`redis`](../redis), [`sqldb`](../sqldb)), and `scheduler.WithHistoryStorage` moves only the history
here. The caller owns the connection, typically opened with [`infrastructure/clickhouse/factory`](../../../../infrastructure/clickhouse/factory).

## Options

| Option          | Default             | Description                                                                 |
|-----------------|---------------------|-----------------------------------------------------------------------------|
| `WithTableName` | `scheduler_history` | Table for execution history; a plain identifier                             |
| `WithEngine`    | `MergeTree`         | Engine rendered by `SchemaDDL`; a MergeTree-family engine, literal params   |
| `WithCluster`   | —                   | Adds `ON CLUSTER` to the DDL and to `DeleteHistory`; pair with Replicated\* |
| `WithTTL`       | `168h`              | Table TTL after the end of a run; non-positive keeps history indefinitely   |

Names are validated rather than escaped: anything else fails with `ErrInvalidIdentifier` or `ErrInvalidEngine`.

## Key types

| Symbol                    | Description                                                                         |
|---------------------------|-------------------------------------------------------------------------------------|
| `New(conn, opts...)`      | Builds the storage over a `Conn` (a subset of `driver.Conn`); performs no I/O       |
| `Storage.EnsureSchema`    | Creates the table from `SchemaDDL` unless it exists                                 |
| `Storage.AddHistory`      | Asynchronous insert that waits for the flush                                        |
| `Storage.History`         | One task's entries, `started_at DESC, id DESC`                                      |
| `Storage.HistoryPaginated`| Keyset page on `(started_at, id)` with a CEL filter over `HistoryFilterFields`      |
| `Storage.DeleteHistory`   | Lightweight `DELETE` with `lightweight_deletes_sync = 2` (ClickHouse 24.x); called by `Scheduler.Unregister` |
| `Storage.CleanupHistory`  | No-op: the table TTL is the retention                                               |
| `SchemaDDL(...)`          | The `CREATE TABLE IF NOT EXISTS` statement, for migrations                          |

## Schema

| Aspect       | Value                                                                                     |
|--------------|-------------------------------------------------------------------------------------------|
| Columns      | `id`, `task_id`, `run_id`, `error` (`String`); `started_at`, `ended_at`, `duration_ms` (`Int64`); `success` (`Bool`) |
| Partition    | `toYYYYMM(toDateTime(ended_at))`                                                          |
| Sorting key  | `(task_id, started_at, id)` — every read is bounded to one task and walks it by start time |
| TTL          | `toDateTime(ended_at) + INTERVAL <WithTTL> SECOND`; omitted for a non-positive TTL        |

Timestamps are Unix seconds, as in `scheduler.TaskHistory`.

## Retention

The table TTL replaces `CleanupHistory`, so the scheduler's `WithHistoryRetention` does not apply to this backend. ClickHouse deletes expired rows
as it merges parts, so an expired entry stays visible until then; a row already past its TTL when written is dropped on insert. The TTL is fixed
when the table is created — change it with `ALTER TABLE … MODIFY TTL`.

## Filters

Filters are translated by [`data/filter/translators/clickhouse`](../../../../data/filter/translators/clickhouse). Its `size()` is `length()`, which
measures a string in bytes, not code points as the other backends do.

## Usage

```go
history, err := clickhouse.New(conn, clickhouse.WithTTL(30*24*time.Hour))
if err != nil {
	return err
}
if err := history.EnsureSchema(ctx); err != nil {
	return err
}
s := scheduler.New(tasks, scheduler.WithHistoryStorage(history))
```

The contract suite runs against a live server in [`tests/integration/schedulerit`](../../../../tests/integration/schedulerit).
