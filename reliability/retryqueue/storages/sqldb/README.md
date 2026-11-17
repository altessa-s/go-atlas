# sqldb

```go
import retrysql "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
```

SQL-backed [`retryqueue.Storage`](../../storage.go) and `retryqueue.Mover` through the standard `database/sql` package: one row per item, keyed by the
item key, the payload `P` encoded by a codec. The caller owns the `*sql.DB` and picks the driver; this package imports no SQL driver. It passes the same
[conformance kit](../storagetest) as the [MongoDB store](../mongo); integration tests run in `tests/integration/retryqueueit`.

## Dialects

| Dialect           | Servers                   | Typical driver                                                  |
|---------------------|-----------------------------|-------------------------------------------------------------------|
| `DialectPostgres` | PostgreSQL 12+            | `github.com/jackc/pgx/v5/stdlib` (`"pgx"`), `github.com/lib/pq` |
| `DialectMySQL`    | MySQL 8.0+, MariaDB 10.6+ | `github.com/go-sql-driver/mysql` (`"mysql"`)                    |

## Key types

| Symbol                             | Description                                                                                  |
|------------------------------------|----------------------------------------------------------------------------------------------|
| `Store[P]`                         | The storage over a `*sql.DB`                                                                 |
| `New[P](db, dialect, opts...)`     | Constructs a store; `ErrUnsupportedDialect`, `ErrInvalidTableName`; performs no I/O          |
| `(*Store[P]).EnsureSchema`         | Creates the table and its indexes; idempotent, safe to run concurrently                      |
| `(*Store[P]).Move`                 | `retryqueue.Mover`: renames the primary key in one `UPDATE`                                  |
| `(*Store[P]).PurgeExpired`         | Deletes resolved items past the retention, against the database clock; returns the count     |
| `(*Store[P]).Close`                | Stops the background purge of `WithPurgeInterval`, waiting within `ctx`; idempotent          |
| `WithTx(ctx, tx)`                  | Runs every method called with the context on the caller's transaction                        |
| `ErrValueTooLong`                  | `Mark`, `Reopen` or `Move` with a key longer than `MaxKeyLength` (255) characters            |

## Options

| Option                      | Default           | Description                                                                                |
|-----------------------------|-------------------|--------------------------------------------------------------------------------------------|
| `WithTableName`             | `retry_items`     | Items table; may be qualified as `schema.name` (≤ 63 chars per part)                       |
| `WithCodec`                 | JSON              | go-atlas-core `serializer.Serializer` for the payload column                               |
| `WithResolvedRetention`     | off               | How long `PurgeExpired` keeps a resolved item; off keeps resolved items until deleted      |
| `WithPurgeInterval`         | 1 minute          | Runs `PurgeExpired` in the background at this interval (needs a retention)                 |
| `WithPurgeBatchSize`        | 1000              | Maximum rows per purge statement                                                           |
| `WithLogger`                | discard           | Receives the background purge's failures, recovered panics and counts                      |

## Table

| Column                                                  | PostgreSQL         | MySQL / MariaDB   | Notes                                         |
|-----------------------------------------------------------|----------------------|---------------------|-------------------------------------------------|
| `id`                                                    | `VARCHAR(255) "C"` | `VARBINARY(1020)` | `Item.Key`; byte-wise order                   |
| `generation`, `attempts`                                | `BIGINT`           | `BIGINT`          |                                               |
| `state`                                                 | `VARCHAR(16) "C"`  | `VARBINARY(16)`   | `pending`, `expired`, `resolved`              |
| `since`, `next_attempt_at`, `expired_at`, `resolved_at` | `BIGINT`           | `BIGINT`          | Caller instants in Unix nanoseconds, 0 = zero |
| `flagged`                                               | `BOOLEAN`          | `BOOLEAN`         |                                               |
| `flag_reason`                                           | `BYTEA`            | `LONGBLOB`        | Bytes: PostgreSQL `TEXT` rejects NUL          |
| `payload`                                               | `BYTEA`            | `LONGBLOB`        | Encoded by the codec                          |

The flag field names and BSON layout options of the MongoDB store are representation details with no SQL counterpart; the observable behavior is the
same.

## Transactions

Every method runs on the transaction the context carries, so a queue transition commits with the business write it belongs to:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
	return err
}
if err := queue.Move(retrysql.WithTx(ctx, tx), oldKey, newKey); err != nil {
	_ = tx.Rollback()
	return err
}
return tx.Commit()
```

`Move` looks up `from` (`ErrItemNotFound`) and `to` (`ErrItemExists`) before writing, so a refusal leaves the transaction usable; only a concurrent
insert of `to` between the check and the `UPDATE` fails the rename with a duplicate key, reported as `ErrItemExists`, which aborts a PostgreSQL
transaction. `Mark` absorbs a taken key without an error on PostgreSQL (`ON CONFLICT DO NOTHING`) and, on MySQL, rolls back only its own statement; its
read-back is a locking read on MySQL so a transaction whose snapshot predates the winner still sees it.

## Retention

MongoDB purges resolved items with a TTL index; SQL has none. With `WithResolvedRetention`, either call `PurgeExpired(ctx)` periodically (once per
sweep, or from your scheduler), or add `WithPurgeInterval(d)` and let the store run it: the loop starts with the first write call (`New` starts no
goroutine), logs failures to `WithLogger`, recovers a panicking purge and keeps going, and stops on `Close(ctx)`, which waits for a purge in progress
within `ctx` and is idempotent. Each purge deletes, in batches bounded by `WithPurgeBatchSize` (default 1000 rows), the resolved items whose
`ResolvedAt` is older than the retention, compared against the database clock at whole seconds, so replicas with skewed clocks agree. An item resolved
at the zero time is never purged. Without a retention it does nothing.

`WithPurgeInterval(0)` disables automatic purging while manual calls stay available. `PurgeExpired(ctx) (int64, error)` returns completed deletions even
if a later batch fails. Background passes use the database outside any caller transaction. Close permanently stops the loop and waits within the
context, including on repeated calls; an exited loop satisfies an ended context.

## Query audit

| Query                                       | Predicate / sort                                       | Index                                 |
|---------------------------------------------|--------------------------------------------------------|---------------------------------------|
| `Mark`, `Reopen`, transitions, `Delete`     | `id` (+ `state` / `generation` / `since`)              | primary key                           |
| `Move`                                      | `id`                                                   | primary key                           |
| `ListDue`                                   | `state = pending AND next_attempt_at <= ?`, sorted     | `<table>_due_idx`                     |
| `WakeDue`                                   | `state = pending AND next_attempt_at > ?`              | `<table>_due_idx`                     |
| `ListByState`                               | `state = ?` sort `since, id`                           | `<table>_state_idx`                   |
| `Requeue`                                   | `state = expired [AND id IN (…)]`                      | `<table>_state_idx` / primary key     |
| `PurgeExpired`                              | `state = resolved AND resolved_at <= db now − ret`     | `<table>_resolved_idx`                |

## Usage

```go
queue, err := retrysql.New[Valuation](db, retrysql.DialectPostgres, retrysql.WithResolvedRetention(7*24*time.Hour))
if err != nil {
    return err
}
if err := queue.EnsureSchema(ctx); err != nil {
    return err
}
```
