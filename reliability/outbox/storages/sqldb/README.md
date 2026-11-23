# sqldb

```go
import outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
```

Package `sqldb` implements `outbox.Storage` on a SQL database through the standard `database/sql` package. The caller owns the `*sql.DB` and picks the
driver; this package imports no SQL driver. It mirrors the [MongoDB store](../mongo): database-clock time predicates, lock-token fencing, and the
same lifecycle.

## Dialects

| Dialect           | Servers                   | Typical driver                                                  |
|-------------------|---------------------------|-----------------------------------------------------------------|
| `DialectPostgres` | PostgreSQL 12+            | `github.com/jackc/pgx/v5/stdlib` (`"pgx"`), `github.com/lib/pq` |
| `DialectMySQL`    | MySQL 8.0+, MariaDB 10.6+ | `github.com/go-sql-driver/mysql` (`"mysql"`)                    |

Both need `FOR UPDATE SKIP LOCKED`, which sets the version floors. On MySQL any `parseTime`/`loc` DSN setting works — see [Server clock](#server-clock).

### PostgreSQL connection loss

The store uses the application's `*sql.DB`. For a standalone PostgreSQL statement it leases a connection until the result is consumed.
If the driver returns `driver.ErrBadConn` or marks a failure as `SafeToRetry` before returning any rows, the store discards that connection
and attempts the statement once more on a different connection. This also covers pgx stdlib errors while preloading the first row of a cached
query. A canceled context, an ambiguous write, a scan or iteration error, and an error inside an explicit transaction go back to the caller.
Audit, saga and distributed-lock SQL stores use the same policy for their standalone PostgreSQL statements.

By default pgx stdlib pings on reuse when more than a second has elapsed since the last session reset. The application that opens the pool can
choose a threshold with `stdlib.OpenDB`.

```go
db := stdlib.OpenDB(*config, stdlib.OptionShouldPing(func(_ context.Context, p stdlib.ShouldPingParams) bool {
    return p.IdleDuration > idlePingThreshold
}))
```

A zero threshold checks every reused connection and adds a round trip per checkout.

## Options

| Option              | Default                | Description                                                                                  |
|---------------------|------------------------|----------------------------------------------------------------------------------------------|
| `WithTableName`     | `events_outbox`        | Events table; may be qualified as `schema.name` (≤ 63 chars per part)                        |
| `WithNotify`        | off                    | Every save runs `pg_notify` on the notification channel in its transaction (PostgreSQL only) |
| `WithNotifyChannel` | derived from the table | Enables `WithNotify` on the given channel: a plain identifier of at most 63 characters       |
| `WithListener`      | none                   | `Listener` that `Watch` subscribes through (PostgreSQL only); nil or a typed nil is none     |
| `WithLogger`        | discard                | Logger `Watch` reports failed subscriptions on                                               |

On MySQL `WithNotify`, `WithNotifyChannel` and `WithListener` fail `New` with `ErrNotifyUnsupported`; an invalid channel fails with
`ErrInvalidNotifyChannel`.

`New` performs no I/O. `EnsureSchema(ctx)` creates the table and its indexes if they do not exist; it is idempotent, bounded by `ctx`, and safe to run
from several instances at once (on PostgreSQL the DDL runs in one transaction under an advisory lock on the table name; on MySQL the indexes are
declared inline). Call it once at startup, or apply the same DDL through your migration tool:

```go
store, err := outboxsql.New(db, outboxsql.DialectPostgres)
if err != nil {
	return err
}
if err := store.EnsureSchema(ctx); err != nil {
	return err
}
```

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
spans several statements. With `WithNotify`, each chunk inserts and signals in one CTE statement, so a single chunk needs no explicit transaction.

## Server clock

All time-based predicates — retry backoff, lock expiry, retention, expiration — compare against the **database clock** (`now()` on PostgreSQL,
`UTC_TIMESTAMP(6)` on MySQL), and the timestamps they compare against (`locked_on`, `last_attempt_on`, `next_attempt_at`, `published_at`) are written
from it. Durations, never client instants, cross the process boundary, so instances with skewed clocks agree on what is due, stuck or expired.

`created_at` and `expires_at` come from the same clock when the caller leaves them unset. The store implements `outbox.BackendClockSaver`, so
`Outbox.Save` hands it the default TTL as a duration: an event without a `CreatedAt` is inserted with the database's `now()`, and one without an
`ExpiresAt` with that clock plus the TTL. On PostgreSQL `now()` is the start of the transaction, so an event saved inside a business transaction is
dated when that transaction began. An explicit `CreatedAt` or `ExpiresAt` is part of the `Event` contract and is stored as given — a default expiry
then counts from the explicit `CreatedAt`. After `Save`, an event whose instants the database stamped still has them zero in the caller's copy.

MySQL `DATETIME(6)` columns hold UTC. Client-supplied instants (`CreatedAt`, `ExpiresAt`) are bound as UTC strings and read back through `DATE_FORMAT`,
never as `time.Time`: go-sql-driver/mysql converts a bound `time.Time` into its `loc` and parses `DATETIME` in it, which would shift every instant for a
DSN with `loc` other than UTC.

## Locking and fencing

`FetchUnprocessedEvents` selects ready events oldest first (`ORDER BY created_at, seq`) `FOR UPDATE SKIP LOCKED` and marks them `in-progress` with a
fresh `lock_token` in one transaction, so concurrent dispatchers take disjoint batches without waiting on each other. Pending events and failed events
whose backoff elapsed are read by one statement each and merged: with the status fixed, the `(status, created_at, seq)` index returns rows in order and
each statement stops at its limit, where a single statement over both statuses would sort the whole backlog first — and on MySQL lock every row it
sorted. A statement may lock up to one batch more than is taken; those rows are released when the fetch transaction ends. `UpdateEvents` writes an event
only while its stored token still matches; a dispatcher whose lease the unlock sweeper reclaimed writes nothing. Unlocking and expiring clear the token.

`UpdateEvents` and `UnlockStuckEvents` take their row locks in primary-key order, so they cannot deadlock on the rows they share. If the database
reports a deadlock or a serialization failure anyway (SQLSTATE `40P01` or `40001`, MySQL error 1213), the statement is retried up to three times with a
short jittered backoff. The package imports no driver, so the code is read from the error's `SQLState()` method (pgx, lib/pq) or `Number` field
(go-sql-driver/mysql).

`DeleteProcessedEvents` deletes and `ExpireEvents` marks 1000 rows at a time, each chunk in its own short transaction, until a chunk comes back short;
a sweep whose context ends keeps what it already did.

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

On MySQL every string column except the fixed-vocabulary `status` is a binary type, so storage and comparison never depend on the database's default
character set.

| Index (`<table>_<suffix>_idx`) | Columns                   | Serves                                                                    |
|--------------------------------|---------------------------|---------------------------------------------------------------------------|
| `fetch`                        | `status, created_at, seq` | The claim, one status at a time, in its `ORDER BY` order (no sort); stats |
| `locked`                       | `status, locked_on`       | The unlock sweep                                                          |
| `published`                    | `status, published_at`    | Retention                                                                 |
| `expires`                      | `status, expires_at`      | The expiry sweep                                                          |

`next_attempt_at` has no index of its own: it is only ever a filter on the claim, whose `created_at` order the `fetch` index already supplies.

## Watch

`database/sql` has no notification API, so the store takes PostgreSQL `LISTEN` from the caller through a `Listener`:

```go
type Listener interface {
	Listen(ctx context.Context, channel string) (<-chan struct{}, error)
}
```

`Listen` returns once the subscription is active; its channel receives a value per notification (a burst may collapse into one) and closes when the
subscription ends. A successful `Listen` must return a non-nil channel; nil is retried as a closed subscription. With `WithListener` the store
implements `outbox.Watcher`: `Store.Watch` subscribes to `NotifyChannel()`, wakes the dispatcher per signal, and resubscribes with an exponential
backoff (1s up to 30s) when the subscription fails or ends, followed by one synthetic wake-up for what it missed. Without a listener `Outbox.Watch`
returns `outbox.ErrWatchUnsupported` and dispatch runs on the poll schedule, which stays configured in every case.

The sending side is `WithNotify`: each insert chunk uses `WITH ins AS (INSERT ... RETURNING 1) SELECT pg_notify(?, '') FROM ins LIMIT 1`, binding the
channel last. The insert and signal share one statement, so a single chunk needs no explicit transaction. A caller transaction from `WithTx` still
controls commit, and larger batches use the store's own transaction. The signal arrives on commit and never for a rollback. Every process writing the
table signals every listener. The default channel is the lower-case table name with a schema qualifier joined by `_` (`app.Events` → `app_events`).
`WithNotifyChannel` overrides it; names are case-sensitive because `pg_notify` does not fold case. Quote mixed-case channel names in psql `LISTEN`.

PostgreSQL serializes the commits of all transactions that issued `NOTIFY` on a database-wide notify-queue lock. Under high write rates, notify costs
commit throughput; polling (`watch: polling`) is the fallback.

An adapter over pgx, one dedicated connection per subscription:

```go
type PGListener struct{ DSN string }

func (l PGListener) Listen(ctx context.Context, channel string) (<-chan struct{}, error) {
	conn, err := pgx.Connect(ctx, l.DSN)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		_ = conn.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	signals := make(chan struct{}, 1)
	go func() {
		defer close(signals)
		defer conn.Close(context.WithoutCancel(ctx))
		for {
			if _, err := conn.WaitForNotification(ctx); err != nil {
				return // ctx ended or the connection broke: Store.Watch resubscribes.
			}
			select {
			case signals <- struct{}{}:
			default: // A signal is already pending.
			}
		}
	}()
	return signals, nil
}

store, err := outboxsql.New(db, outboxsql.DialectPostgres, outboxsql.WithNotify(), outboxsql.WithListener(PGListener{DSN: dsn}))
```

## Factories

[`factory/reliability/outbox`](../../../../factory/reliability/outbox) and [`factory/messaging/broker`](../../../../factory/messaging/broker) build an
outbox over this store when `outbox.storage.type` is `sqldb`, over the handle given to `UseSQLDB`. They run `EnsureSchema` only with
`storage.sqldb.ensureSchema: true` (default `false`); otherwise the table must already exist. With `outbox.watch: notify` they enable `WithNotify`,
take the listener from `UseSQLListener`, and start the watcher.

## Testing

The [`storagetest`](../storagetest) contract suite and the outbox integration scenarios in
[`tests/integration/outboxit`](../../../../tests/integration/outboxit) run against PostgreSQL, MariaDB and MySQL as well as MongoDB; the `Watch`
contract runs on PostgreSQL through a pgx listener.
