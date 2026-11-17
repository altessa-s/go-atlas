# sqldb

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
```

Package `sqldb` implements [`storages.Storage`](../README.md) on PostgreSQL, MySQL and MariaDB through `database/sql`. It stores state and replay bytes
unchanged, acquires absent or expired keys atomically, and compares live stored bytes for `Complete`, `Steal` and `Release` like the [MongoDB
backend](../mongo/README.md).

## Public API

| API                                                  | Description                                                                        |
|------------------------------------------------------|------------------------------------------------------------------------------------|
| `New(db *sql.DB, dialect Dialect, opts ...Option)`   | Validates and constructs a `*Storage` without I/O or goroutines                    |
| `DialectPostgres` / `DialectMySQL`                   | PostgreSQL 12+ / MySQL 8.0+ and MariaDB 10.6+                                      |
| `EnsureSchema(ctx)`                                  | Creates the table and expiry index separately from construction                    |
| `WithTx(ctx, tx)`                                    | Joins the caller's transaction for every key operation                             |
| `Close(ctx) error`                                   | Permanently stops cleanup and waits within the context; leaves the database open   |
| `ErrRestartTransaction`                              | Matchable signal to roll back and retry the whole caller-owned transaction         |
| `PurgeExpired(ctx) (int64, error)`                   | Drains expired rows in bounded batches and returns the total                       |
| `MaxKeyLength`                                       | 255 characters, including the Keeper's composed namespace                          |

The caller opens and closes `*sql.DB` and registers its driver. No SQL driver is imported by this package or required by the root module. Oversized keys
fail with `ErrValueTooLong` before SQL execution; they are never truncated or hashed. PostgreSQL keys are text; MySQL uses `VARBINARY(1020)` for up to
255 UTF-8 characters, preserving case and trailing spaces. Values use `BYTEA` or `LONGBLOB` for binary-safe CAS and replay.

## Options

| Option                 | Default              | Description                                                                                  |
|------------------------|----------------------|----------------------------------------------------------------------------------------------|
| `WithTableName`        | `idempotency_keys`   | Plain identifier, optionally qualified by one schema name; each part at most 63 characters   |
| `WithPurgeInterval`    | 1 minute             | Sweep interval; starts on the first write, zero disables                                     |
| `WithPurgeBatchSize`   | 1000                 | Maximum rows per purge statement                                                             |
| `WithLogger`           | Discard              | Receives cleanup errors and recovered panics                                                 |
| `WithTTL`              | 24h                  | Result lifetime and default lock lifetime; non-positive option values keep the default       |

`AttemptLockWithTTL` accepts a positive per-call lock TTL; zero or negative falls back to `WithTTL`. `Complete` and `Steal` reset expiry using the
configured TTL. The storage port has no per-call result-TTL parameter. PostgreSQL uses `statement_timestamp()` so expiry advances inside a business
transaction, and MySQL uses `UTC_TIMESTAMP(6)`. Application clock skew cannot extend a lock.

## Usage

```go
store, err := sqldb.New(db, sqldb.DialectPostgres,
    sqldb.WithTableName("command_keys"), sqldb.WithTTL(24*time.Hour))
if err != nil {
    return err
}
if err := store.EnsureSchema(ctx); err != nil {
    return err
}
keeper := idempotency.New(store, idempotency.WithMaxLockDuration(time.Minute))
```

PostgreSQL schema creation uses the shared advisory-lock DDL helper. MySQL creates indexes inline with `CREATE TABLE IF NOT EXISTS`; apply migrations to
repair an existing table's missing index. `EnsureSchema` runs directly on the database and does not join `WithTx`.

## Acquisition and ownership

Acquisition first inserts with `ON CONFLICT DO NOTHING` (PostgreSQL) or `INSERT IGNORE` (MySQL), then conditionally updates an expired row. A successful
write reports acquisition even when competing callers supply identical bytes. A losing caller reads the current live value; if deletion or expiry raced
that read, acquisition retries until it obtains a result or the context ends. In a transaction the read is locking, avoiding stale MySQL snapshots.

CAS tokens are copies of the stored lock bytes, matching MongoDB. `Complete` with a nil token unconditionally inserts or overwrites; the higher-level
Keeper requires lock state. `Release` rejects empty tokens. Missing, expired and mismatched CAS operations return `storages.ErrLockStolen`. Changing a
revision column ensures a matching MySQL update succeeds even when its bytes and expiry are unchanged at the database's timestamp precision.

## Transactions

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    return err
}
defer func() { _ = tx.Rollback() }()
txCtx := sqldb.WithTx(ctx, tx)
ok, held, err := keeper.AttemptLock(txCtx, key)
if err != nil || !ok {
    return err
}
if err := applyCommand(txCtx, tx); err != nil {
    return err
}
if err := keeper.Complete(txCtx, key, result, held); err != nil {
    return err
}
return tx.Commit()
```

All key operations, including cleanup, use the supplied transaction. An abort rolls back locks, results and business writes together. An uncommitted
insert may block duplicate requests until commit. Within `WithTx`, MariaDB error 1020 (a stale snapshot under `innodb_snapshot_isolation`),
MySQL/MariaDB deadlock 1213 and PostgreSQL SQLSTATE 40001/40P01 return an error matching `ErrRestartTransaction`. The original driver error remains
available through `errors.Is`/`errors.As`. Roll back and retry the whole business transaction; the storage never retries statements inside it. MariaDB
can require this restart after another connection changes a row observed in a REPEATABLE READ snapshot. MySQL and PostgreSQL locking reads can instead
observe current bytes. Outside `WithTx`, database errors retain their original behavior, without automatic retries or the restart sentinel. A nil
`WithTx` transaction falls back to `*sql.DB`.

## Expired-row cleanup

Background cleanup defaults to one minute (`sqldb.purgeInterval`, zero disables). It starts on the first key write and uses the database directly,
independent of request cancellation and WithTx. Errors and recovered panics are logged; future passes continue. Call Close before closing the database.
Explicit PurgeExpired(ctx) calls join WithTx and drain all expired keys using WithPurgeBatchSize. Acquisition and CAS enforce expiry independently of
cleanup. PostgreSQL uses FOR UPDATE SKIP LOCKED; MySQL uses a bounded DELETE. Neither deletes a live key.

`PurgeExpired(ctx) (int64, error)` returns completed deletions even if a later batch fails. Close permanently stops the loop and waits within the
context, including on repeated calls; an exited loop satisfies an ended context.

## Tests

Unit tests use the shared fake SQL driver to verify query shape, validation, TTL binding, CAS results and transaction routing. Live conformance, replay
records, concurrency, expiry, transaction rollback and factory scenarios run in `tests/integration/idempotencyit` against all three SQL servers:

```sh
go test -C tests/integration -race -count=1 -p 1 -v ./idempotencyit/...
```

Inspect verbose output for `--- SKIP`: unreachable backends skip and must not be reported as passed.
