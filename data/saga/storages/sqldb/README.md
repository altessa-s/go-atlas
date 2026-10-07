# sqldb

```go
import sagasql "github.com/altessa-s/go-atlas/data/saga/storages/sqldb"
```

Durable [`saga.Storage`](../../storage.go) on a SQL database through `database/sql`: PostgreSQL 12+ and MySQL 8.0+ / MariaDB 10.6+. The caller owns
the `*sql.DB` and chooses the driver; the toolkit depends on no SQL driver. Each saga instance is one row keyed by its ID, and the row's `version`
column is the optimistic-concurrency token, so `Update` rejects a stale writer with `errs.ErrVersionConflict`.

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
| `Create`           | PostgreSQL: `INSERT … ON CONFLICT (id) DO NOTHING`. MySQL: plain `INSERT`; a failure with the row present is `ErrInstanceExists`. |
| `Update`           | `UPDATE … WHERE id = ? AND version = ?`; on no match a lookup tells `ErrInstanceNotFound` from `ErrVersionConflict`.              |
| `FetchRecoverable` | One indexed `SELECT` mirroring `Instance.Recoverable`, compared at full precision against the given `now`.                        |
| `Delete`           | `DELETE … WHERE id = ?`; a missing row is not an error.                                                                           |

## Schema

Timestamps are `BIGINT` Unix nanoseconds (`0` for the zero time), so `LeaseUntil` round-trips at full precision. `pending_steps` and `steps` hold
JSON, `data` the opaque payload. On MySQL/MariaDB every string column is a binary type (`VARBINARY`, `MEDIUMBLOB`, `LONGBLOB`), so IDs compare
byte-wise whatever the server's character set; on PostgreSQL the ID is `VARCHAR(255) COLLATE "C"`. Two indexes back the recovery scan:
`(status, lease_until)` and `(status, deadline)`. On PostgreSQL `EnsureSchema` runs in one transaction under an advisory lock on the table name.

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

## See also

- [storages/storagetest](../storagetest) — the contract suite this backend passes.
- [storages/mongo](../mongo) — the MongoDB backend with the same default collection name.
- [service/scheduler/storages/sqldb](../../../../service/scheduler/storages/sqldb) and [data/outbox/storages/sqldb](../../../outbox/storages/sqldb) —
  the other SQL backends.
