# sqldb

```go
import auditsql "github.com/altessa-s/go-atlas/data/audit/storages/sqldb"
```

`audit.Storage` on PostgreSQL 12+ and MySQL 8.0+ / MariaDB 10.6+ through `database/sql`, for deployments that keep audit events in the relational
database they already run; [clickhouse](../clickhouse) remains the choice for analytical volumes. The caller owns the `*sql.DB` and registers the
driver.

## Constructors

| Function                    | Description                                                                 |
|-----------------------------|-----------------------------------------------------------------------------|
| `New(db, dialect, opts...)` | Storage over `db` for `DialectPostgres` or `DialectMySQL`; performs no I/O  |
| `Storage.EnsureSchema(ctx)` | Creates the table and its indexes if absent; idempotent and concurrent-safe |

## Options

| Option             | Default        | Description                                                        |
|--------------------|----------------|--------------------------------------------------------------------|
| `WithTableName`    | `audit_events` | Table name, optionally schema-qualified                            |
| `WithMaxBatchRows` | 500            | Rows per INSERT; a larger `StoreBatch` is split in one transaction |

## Semantics

| Aspect     | Behavior                                                                                                                     |
|------------|------------------------------------------------------------------------------------------------------------------------------|
| Row        | ID, `ts_ms`, the queryable fields as columns, the whole event as a JSON `payload`                                            |
| StoreBatch | Atomic: one INSERT, or chunked INSERTs in one transaction                                                                    |
| Replay     | Idempotent: an already stored ID is skipped (`ON CONFLICT DO NOTHING` / no-op `ON DUPLICATE KEY UPDATE`), so retries succeed |
| Order      | `(ts_ms, id)` with the ID compared byte-wise, as the page tokens of `audit.FetchPage` require                                |
| Time range | `StartTime`/`EndTime` bound whole milliseconds; returned events keep their full-precision timestamp                          |
| Metadata   | Round-trips through JSON: numbers come back as `float64`                                                                     |
| Retention  | None built in: delete old rows or partition the table through the database                                                   |
| Close      | No-op; the `*sql.DB` belongs to the caller                                                                                   |

Indexes: `(ts_ms, id)`, `(actor_id, ts_ms, id)`, `(resource_type, resource_id, ts_ms, id)`, `(request_id)`, `(trace_id)`. On MySQL/MariaDB the
filter columns are `MEDIUMBLOB` indexed by a 255-byte prefix.

## See also

- [storages/storagetest](../storagetest) — the conformance suite this storage passes.
