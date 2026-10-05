# sqldb

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"
```

Package `sqldb` implements `scheduler.Storage` on a SQL database through the standard `database/sql` package. The caller owns the `*sql.DB` and picks
the driver; go-atlas itself depends on no SQL driver. Task states and execution history live in two tables, and CEL filters are evaluated by the
database.

## Dialects

| Dialect           | Servers                      | Typical driver                                                  |
|-------------------|------------------------------|-----------------------------------------------------------------|
| `DialectPostgres` | PostgreSQL 12+               | `github.com/jackc/pgx/v5/stdlib` (`"pgx"`), `github.com/lib/pq` |
| `DialectMySQL`    | MySQL 8.0.17+, MariaDB 10.6+ | `github.com/go-sql-driver/mysql` (`"mysql"`)                    |

PostgreSQL databases must use UTF-8 encoding.

## Options

| Option             | Default             | Description                                              |
|--------------------|---------------------|----------------------------------------------------------|
| `WithTasksTable`   | `scheduler_tasks`   | Table for task states; may be qualified as `schema.name` |
| `WithHistoryTable` | `scheduler_history` | Table for execution history; may be schema-qualified     |

Table names must be plain identifiers (`[A-Za-z_][A-Za-z0-9_]*`, optionally one `schema.` qualifier); they are the only SQL fragments that cannot be
bound as parameters, so anything else is rejected with `ErrInvalidTableName`.

## Schema

`EnsureSchema` creates both tables and their indexes if they do not exist and is idempotent. Call it once at startup, or apply the same DDL through
your migration tool — `New` performs no I/O and the factory never runs DDL, matching the MongoDB backend's `EnsureIndexes`.

It also upgrades a tasks table created by an earlier release, adding the run-ownership columns `run_lease_until BIGINT NOT NULL DEFAULT 0`,
`run_lease_id` (typed like `last_run_id`, default `''`) and `run_at BIGINT NOT NULL DEFAULT 0`. PostgreSQL uses `ADD COLUMN IF NOT EXISTS`; MySQL 8
has no such clause, so on MySQL/MariaDB the missing columns are read from `information_schema` first. Several instances may run the upgrade at
once. Applying the DDL through migrations instead, add the same three columns.

| Table   | Indexes                                                                                                  |
|---------|----------------------------------------------------------------------------------------------------------|
| tasks   | primary key `id`; `(status, next_run_at)` for due-task scans                                             |
| history | primary key `id`; `(task_id, started_at DESC, id DESC)` for both history reads; `(ended_at)` for cleanup |

Index names derive from the table name and stay within the 63-character identifier limit (long names end in a hash of the table name).

### Exact string identity

Every string column compares exactly — no trailing-space padding, no case folding — so `task` and `task ` are distinct tasks and run ownership
fences are case-sensitive, like the memory backend. On PostgreSQL IDs use `COLLATE "C"` (byte order). On MySQL/MariaDB every string column is
declared `utf8mb4` with a **NO PAD binary collation**, chosen per engine because the name differs: `utf8mb4_0900_bin` on MySQL (hence 8.0.17+) and
`utf8mb4_nopad_bin` on MariaDB. `EnsureSchema` probes `VERSION()` once and returns `ErrUnsupportedVersion` for older servers. `utf8mb4_bin` is never
used: it is `PAD SPACE`. The schema never inherits the database's default character set or collation.

### Value limits

Task, history and run IDs hold at most `MaxIDLength` (255) characters and a schedule at most `MaxScheduleLength` (1024). Longer values are rejected
with `ErrValueTooLong` before any SQL runs: PostgreSQL silently trims excess trailing spaces, which would otherwise make `id` and `id ` past the
limit the same row.

## Atomicity

| Method          | Implementation                                                                                                                                                              |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `UpsertTask`    | One `INSERT … ON CONFLICT` / `ON DUPLICATE KEY UPDATE` that sets `revision = revision + 1` (1 when new)                                                                     |
| `CreateTask`    | PostgreSQL: one `INSERT … ON CONFLICT DO NOTHING`; MySQL: one plain `INSERT`, a failure reported as `false` when the task exists                                            |
| `ClaimRun`      | One conditional `UPDATE` matching `status = active`, `run_started_at = 0` and, when fenced, `next_run_at` and `run_at`; stores the first lease                              |
| `RenewRun`      | One conditional `UPDATE` matching `last_run_id` and an unfinished run (`run_started_at <> 0`); binds the lease to the run                                                   |
| `FinishRun`     | One conditional `UPDATE` matching `last_run_id` and an unfinished run; applies the one-shot executed rule, preserves pause/disable and a changed schedule, clears the lease |
| `ReplaceTaskIf` | One `UPDATE` whose `WHERE` carries the whole `TaskFence`, storing `expect.Revision + 1`                                                                                     |
| `DeleteTask`    | Task and history deleted in one transaction                                                                                                                                 |

Every conditional write also changes `revision`, so "row matched" and "row changed" coincide — the result is correct under MySQL's changed-rows
`RowsAffected` semantics too. MySQL evaluates `SET` assignments left to right; no expression in `FinishRun` reads a column assigned before it.

## Filters

`TasksPaginated` and `HistoryPaginated` translate CEL filters with the [`postgres`](../../../../data/filter/translators/postgres) and
[`mariadb`](../../../../data/filter/translators/mariadb) translators (allow-listed to `TaskFilterFields` / `HistoryFilterFields`) and evaluate them
in the database. Because the database evaluates them, `size()` counts **characters** (`length` / `CHAR_LENGTH`), while the memory backend's
evaluator counts UTF-8 bytes — the results differ for non-ASCII text. `matches()` uses the database's regular-expression engine
(POSIX `~` on PostgreSQL, ICU/PCRE `REGEXP` on MySQL/MariaDB), case-sensitive on both.

## Usage

```go
db, err := sql.Open("pgx", "postgres://user:pass@host/db")
if err != nil {
	return err
}
storage, err := sqldb.New(db, sqldb.DialectPostgres,
	sqldb.WithTasksTable("scheduler_tasks"),
	sqldb.WithHistoryTable("scheduler_history"),
)
if err != nil {
	return err
}
if err := storage.EnsureSchema(ctx); err != nil {
	return err
}
s := scheduler.New(storage)
```

With the factory, set `storage.type: sql` and inject the handle with `UseSQLDB(db)`; see [factory](../../factory). The storage the factory builds
is not exposed, so create the schema beforehand — `EnsureSchema` on a storage from `New` with the same handle, dialect and table names (it holds
no other state), or your migrations.

## Testing

The contract suite in [`storagetest`](../../storagetest) runs against live PostgreSQL, MariaDB and MySQL in
[`tests/integration/schedulerit`](../../../../tests/integration/schedulerit), together with the end-to-end and run-ownership scheduler scenarios and
the upgrade of a tasks table from the previous schema.
