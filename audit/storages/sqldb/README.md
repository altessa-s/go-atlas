# sqldb

```go
import auditsql "github.com/altessa-s/go-atlas/audit/storages/sqldb"
```

`audit.Storage` on PostgreSQL 12+ and MySQL 8.0+ / MariaDB 10.6+ through `database/sql`, for deployments that keep audit events in the relational
database they already run; [clickhouse](../clickhouse) remains the choice for analytical volumes. The caller owns the `*sql.DB` and registers the
driver.

## Constructors

| Function                      | Description                                                                                      |
|---------------------------------|----------------------------------------------------------------------------------------------------|
| `New(db, dialect, opts...)`   | Storage over `db` for `DialectPostgres` or `DialectMySQL`; no I/O, no goroutine                  |
| `Storage.EnsureSchema(ctx)`   | Creates the table and its indexes if absent; idempotent and concurrent-safe                      |
| `Storage.PurgeExpired(ctx)`   | Deletes expired events in bounded batches; returns the deleted count and any error               |

## Options

| Option                   | Default              | Description                                                                                          |
|--------------------------|----------------------|------------------------------------------------------------------------------------------------------|
| `WithTableName`          | `audit_events`       | Table name, optionally schema-qualified                                                              |
| `WithMaxBatchRows`       | 500                  | Rows per INSERT; a larger `StoreBatch` is split in one transaction                                   |
| `WithTTL`                | 0                    | Event retention; zero keeps events forever; nonpositive option values are ignored, as in MongoDB     |
| `WithPurgeInterval`      | 1 minute             | Time between retention passes; zero disables the loop                                                |
| `WithPurgeBatchSize`     | 1000                 | Maximum rows deleted per statement; a pass drains all expired rows in batches                        |
| `WithClock`              | `clock.Real()`       | Application clock for retention cutoffs; interval timers use real time                               |
| `WithLogger`             | discard              | Logger for background purge errors and recovered panics                                              |

## Semantics

| Aspect       | Behavior                                                                                                                       |
|----------------|----------------------------------------------------------------------------------------------------------------------------------|
| Row          | ID, `ts_ms`, the queryable fields as columns, the whole event as a JSON `payload`                                              |
| StoreBatch   | Atomic: one INSERT, or chunked INSERTs in one transaction                                                                      |
| Replay       | Idempotent: an already stored ID is skipped (`ON CONFLICT DO NOTHING` / no-op `ON DUPLICATE KEY UPDATE`), so retries succeed   |
| Order        | `(ts_ms, id)` with the ID compared byte-wise, as the page tokens of `audit.FetchPage` require                                  |
| Time range   | `StartTime`/`EndTime` bound whole milliseconds; returned events keep their full-precision timestamp                            |
| Metadata     | Round-trips through JSON: numbers come back as `float64`                                                                       |
| Retention    | Optional background batched deletes from the first store when TTL is positive; explicit `PurgeExpired` too                     |
| Close        | Stops background retention for good and waits for it within ctx; the `*sql.DB` belongs to the caller                           |

Indexes: `(ts_ms, id)`, `(actor_id, ts_ms, id)`, `(resource_type, resource_id, ts_ms, id)`, `(request_id)`, `(trace_id)`. Each queryable field holds at
most `MaxFilterBytes` (255) bytes and the ID `MaxIDLength` (255) characters: a longer value fails the batch with `ErrValueTooLong` before anything is
written, so an index entry always fits and MySQL never truncates a value. `WithMaxBatchRows` is capped at 5461 rows, PostgreSQL's 65535 bind parameters.

## Retention

`WithTTL(d)` expires events by their stored `ts_ms`, using the application clock minus `d`, consistent with the caller-supplied event timestamps. Rows
exactly at the cutoff are retained. Each pass fixes one cutoff and drains expired rows through the existing `(ts_ms, id)` index: MySQL/MariaDB use
`DELETE ... ORDER BY ts_ms, id LIMIT n`; PostgreSQL deletes IDs selected in timestamp order with `LIMIT n FOR UPDATE SKIP LOCKED` so concurrent purges
skip busy rows. Each batch commits independently, and each statement has a 30-second timeout. `PurgeExpired(ctx)` returns completed deletions even if a
later batch fails or the context ends; with TTL zero it returns `0, nil` without SQL.

Unlike [mongo](../mongo), which delegates expiration to a server TTL index, SQL cleanup runs only while a TTL-enabled storage is open. `New` starts no
goroutine: the loop starts with the first `Store` or `StoreBatch` (reads and empty batches do not start it) and runs until `Close`. The default one-
minute interval limits polling overhead; expiration is asynchronous, so old rows remain queryable until purged. Background failures and panics are
logged and retried on the next pass. Create the schema before the first pass runs. Call `Close(ctx)` to stop the loop for good, cancel its in-flight SQL
and wait within `ctx` for the goroutine to exit; it returns `ctx`'s error if the wait ends first, and a later Close waits again. Explicit `PurgeExpired`
calls remain available after Close for external schedulers, and use their caller's context. Close does not wait for those explicit calls or close the
database. Background passes carry no request values or transactions.

```go
storage, err := auditsql.New(db, auditsql.DialectPostgres,
    auditsql.WithTTL(90*24*time.Hour),
    auditsql.WithLogger(logger),
)
if err != nil {
    return err
}
defer storage.Close(ctx)
if err := storage.EnsureSchema(ctx); err != nil {
    return err
}
```

`WithPurgeInterval(0)` disables the loop while manual purges stay available. Close returns nil once the loop has exited, even with an ended context.

Standalone PostgreSQL statements retry once on a driver-confirmed pre-send failure. Transactions keep their connection and surface the error.
See the [PostgreSQL connection loss policy](../../../reliability/outbox/storages/sqldb/README.md#postgresql-connection-loss) for checkout settings and retry limits.

## See also

- [storages/storagetest](../storagetest) — the conformance suite this storage passes.
