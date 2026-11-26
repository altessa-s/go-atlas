# sqldb

```go
import sagasql "github.com/altessa-s/go-atlas/reliability/saga/storages/sqldb"
```

Durable [`saga.Storage`](../../storage.go) on a SQL database through `database/sql`: PostgreSQL 12+ and MySQL 8.0+ / MariaDB 10.6+. The caller owns the
`*sql.DB` and chooses the driver; this package imports no SQL driver. Each saga instance is one row keyed by its ID, and the row's `version` column is
the optimistic-concurrency token, so `Update` rejects a stale writer with `errs.ErrVersionConflict`.

## Constructors

| Function                    | Description                                                                           |
|-----------------------------|---------------------------------------------------------------------------------------|
| `New(db, dialect, opts...)` | Store over `db` for `DialectPostgres` or `DialectMySQL`; performs no I/O.             |
| `Store.EnsureSchema(ctx)`   | Creates the table and its indexes if absent; idempotent and safe to run concurrently. |

## Options

| Option          | Default          | Description                                                               |
|-----------------|------------------|---------------------------------------------------------------------------|
| `WithTableName` | `saga_instances` | Table name, optionally schema-qualified; a plain SQL identifier per part. |

## Errors

| Error                   | Returned when                                                                            |
|-------------------------|------------------------------------------------------------------------------------------|
| `ErrUnsupportedDialect` | `New` gets a dialect other than `postgres` or `mysql`.                                   |
| `ErrInvalidTableName`   | `New` gets a table name that is not a plain identifier, or a part exceeds 63 characters. |
| `ErrValueTooLong`       | `Create` gets an ID longer than `MaxIDLength` (255) characters.                          |

## Semantics

| Operation          | Implementation                                                                                                                    |
|--------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `Create`           | PostgreSQL: `INSERT … ON CONFLICT (id) DO NOTHING`. MySQL: plain `INSERT`; a duplicate-key error (Number field 1062) is `ErrInstanceExists`.  |
| `Update`           | `UPDATE … WHERE id = ? AND version = ?`; on no match a lookup tells `ErrInstanceNotFound` from `ErrVersionConflict`.              |
| `FetchRecoverable` | `SELECT` mirroring `Instance.Recoverable` at full precision against `now`, in `(status, lease_until)` index order: compensating sagas first. |
| `Delete`           | `DELETE … WHERE id = ?`; a missing row is not an error.                                                                           |
| `DeleteTerminal`   | Selects terminal ids older than the cutoff in batches (`(status, updated_at)` index), then `DELETE … WHERE status IN (…) AND id IN (…)`. |

## Schema

Timestamps are `BIGINT` Unix nanoseconds (`0` for the zero time), so `LeaseUntil` round-trips at full precision. `pending_steps` and `steps` hold JSON,
`data` the opaque payload. On MySQL/MariaDB every string column is a binary type (`VARBINARY`, `MEDIUMBLOB`, `LONGBLOB`), so IDs compare byte-wise
whatever the server's character set; on PostgreSQL the ID is `VARCHAR(255) COLLATE "C"`. Three indexes back the recovery scan and retention: `(status, lease_until)`,
`(status, deadline)` and `(status, updated_at)`. A MySQL or MariaDB table created before `(status, updated_at)` needs it added by hand (`CREATE INDEX
<table>_terminal_idx ON <table> (status, updated_at)`), because `EnsureSchema` is `CREATE TABLE IF NOT EXISTS`; PostgreSQL gets it from `EnsureSchema`. On PostgreSQL `EnsureSchema` runs in one transaction under an advisory lock on the table name.

## Usage

```go
db, _ := sql.Open("pgx", dsn)
store, err := sagasql.New(db, sagasql.DialectPostgres)
if err != nil {
	return err
}
if err := store.EnsureSchema(ctx); err != nil { // or apply the DDL via migrations
	return err
}
orch := saga.New(store, def)
```

Standalone PostgreSQL statements retry once on a driver-confirmed pre-send failure. Transactions keep their connection and surface the error.
See the [PostgreSQL connection loss policy](../../../outbox/storages/sqldb/README.md#postgresql-connection-loss) for checkout settings and retry limits.

## See also

- [storages/storagetest](../storagetest) — the contract suite this backend passes.
- [storages/mongo](../mongo) — the MongoDB backend with the same default collection name.
- [reliability/outbox/storages/sqldb](../../../outbox/storages/sqldb) — another SQL backend.

## Caller transactions

Pass `WithTxResolver` a function that finds the caller's `*sql.Tx` in each operation's context. The caller owns completion and rollback.
Prepare the binding with `pools.Binding(ctx, name)`, then pass its pool and resolver to the factory with `UseSQLPool(db, resolve)`.
