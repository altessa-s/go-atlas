# sqldb

```go
import outboxsql "github.com/altessa-s/go-atlas/data/outbox/store/sqldb"
```

Package `sqldb` implements `outbox.Store` on a SQL database through the standard `database/sql` package. The caller owns the `*sql.DB` and picks the
driver; go-atlas itself depends on no SQL driver. It mirrors the [MongoDB store](../mongo): database-clock time predicates, lock-token fencing, and
the same lifecycle.

## Dialects

| Dialect           | Servers                   | Typical driver                                                  |
|-------------------|---------------------------|-----------------------------------------------------------------|
| `DialectPostgres` | PostgreSQL 12+            | `github.com/jackc/pgx/v5/stdlib` (`"pgx"`), `github.com/lib/pq` |
| `DialectMySQL`    | MySQL 8.0+, MariaDB 10.6+ | `github.com/go-sql-driver/mysql` (`"mysql"`)                    |

Both need `FOR UPDATE SKIP LOCKED`, which sets the version floors. On MySQL any `parseTime`/`loc` DSN setting works — see [Server clock](#server-clock).

## Options

| Option                    | Default         | Description                                                           |
|---------------------------|-----------------|-----------------------------------------------------------------------|
| `WithTableName`           | `events_outbox` | Events table; may be qualified as `schema.name` (≤ 63 chars per part) |
| `WithContext`             | background      | Base context for the schema creation `New` performs                   |
| `WithSchemaCreateTimeout` | `10s`           | Bound on that schema creation                                         |

`New` creates the table and its indexes if they do not exist, like the MongoDB store creating its indexes; the DDL is idempotent.

## Transactions

`Save` must run inside the business transaction — that atomicity is the point of the outbox. Pass the transaction through the context:

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
	return err
}
if err := orders.Insert(ctx, tx, order); err != nil {
	_ = tx.Rollback()
	return err
}
if err := ob.Save(outboxsql.WithTx(ctx, tx), outbox.Event{Key: "orders.created", Payload: payload}); err != nil {
	_ = tx.Rollback()
	return err
}
return tx.Commit()
```

Only `SaveEvents` uses the context transaction. Without one, a batch is still all-or-nothing: a single `INSERT`, or its own transaction when the batch
spans several statements.

## Server clock

All time-based predicates — retry backoff, lock expiry, retention, expiration — compare against the **database clock** (`now()` on PostgreSQL,
`UTC_TIMESTAMP(6)` on MySQL), and the timestamps they compare against (`locked_on`, `last_attempt_on`, `next_attempt_at`, `published_at`) are written
from it. Durations, never client instants, cross the process boundary, so instances with skewed clocks agree on what is due, stuck or expired.

MySQL `DATETIME(6)` columns hold UTC. Client-supplied instants (`CreatedAt`, `ExpiresAt`) are bound as UTC strings and read back through
`DATE_FORMAT`, never as `time.Time`: go-sql-driver/mysql converts a bound `time.Time` into its `loc` and parses `DATETIME` in it, which would shift
every instant for a DSN with `loc` other than UTC.

## Locking and fencing

`FetchUnprocessedEvents` selects ready events oldest first (`ORDER BY created_at, seq`) `FOR UPDATE SKIP LOCKED` and marks them `in-progress` with a
fresh `lock_token` in one transaction, so concurrent dispatchers take disjoint batches without waiting on each other. `UpdateEvents` writes an event
only while its stored token still matches; a dispatcher whose lease the unlock sweeper reclaimed writes nothing. Unlocking and expiring clear the
token.

## Schema

| Column       | PostgreSQL     | MySQL / MariaDB  | Notes                                                                                         |
|--------------|----------------|------------------|-----------------------------------------------------------------------------------------------|
| `id`         | `VARCHAR(255)` | `VARBINARY(255)` | Primary key                                                                                   |
| `seq`        | identity       | `AUTO_INCREMENT` | Insertion order; breaks `created_at` ties (a batch saved together shares one `CreatedAt`)     |
| `topic`      | `BYTEA`        | `LONGBLOB`       | `Event.Key`                                                                                   |
| `payload`    | `BYTEA NULL`   | `LONGBLOB NULL`  | `nil` round-trips as `nil`, empty as empty                                                    |
| `status`     | `VARCHAR(32)`  | `VARCHAR(32)`    |                                                                                               |
| `error`      | `BYTEA NULL`   | `LONGBLOB NULL`  | Unbounded handler errors, NUL bytes included                                                  |
| `lock_token` | `VARCHAR(64)`  | `VARBINARY(64)`  | Fencing token of the current lease                                                            |
| timestamps   | `TIMESTAMPTZ`  | `DATETIME(6)`    | `created_at`, `locked_on`, `last_attempt_on`, `next_attempt_at`, `published_at`, `expires_at` |

Indexes: `(status, created_at, seq)` for fetching, `(status, locked_on)` for the unlock sweep, `(status, published_at)` for retention. MySQL string
columns are binary types, so storage and comparison never depend on the database's default character set.

## Watch

`database/sql` has no notification API, so the store does not implement `outbox.Watcher`; `Outbox.Watch` returns `outbox.ErrWatchUnsupported` and
dispatch runs on the poll schedule.

## Factories

`data/outbox/factory.OutboxBuilder.BuildWithSQLDB(db, dialect, handler)` and
`transport/broker/factory.BrokerBuilder.CreateOutboxWithSQLDB(db, dialect, publisher)` build an outbox over this store.

## Testing

The outbox integration scenarios in [`tests/integration/outboxit`](../../../../tests/integration/outboxit) run against PostgreSQL, MariaDB and MySQL
as well as MongoDB.
