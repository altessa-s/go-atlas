# sqldialect

```go
import "github.com/altessa-s/go-atlas/internal/sqldialect"
```

SQL text helpers shared by the `database/sql` storages — such as [outbox `sqldb`](../../reliability/outbox/storages/sqldb): table-name validation and
quoting, index naming, placeholder binding, and serialized PostgreSQL schema creation. Internal to the module.

## Key types

| Symbol                                         | Description                                                                                  |
|------------------------------------------------|----------------------------------------------------------------------------------------------|
| `Style`                                        | Identifier quote and placeholder form of a SQL flavor: `Postgres` (`"`, `$n`), `MySQL` (`?`) |
| `Style.Table`                                  | Validates `name` or `schema.name` (each part at most `MaxIdentLen` = 63) and quotes it       |
| `Style.IndexName`                              | `<table>_<suffix>_idx`, or a readable prefix plus an FNV-32a hash when over 63 (long suffixes included in the hash)  |
| `Style.Bind`                                   | Rewrites `?` as `$n` from a start index on numbered styles; positional styles pass through   |
| `ExecPostgresDDL`                              | Runs DDL in one transaction under sorted `pg_advisory_xact_lock`s on the table names         |
| `MySQLErrorNumber`                            | Reads the first server error number through wrapped and joined trees without importing a SQL driver              |
| `IsDuplicateKey` | Recognizes PostgreSQL SQLSTATE 23505 and MySQL error number 1062 in error trees |
| `QueryKeys`, `InList` | Read one batch of binary keys; expand a `%s` into an IN list of `n` placeholders |
| `Unqualified`                                  | Table part of a possibly qualified name                                                      |
| `ErrUnsupportedDialect`, `ErrInvalidTableName`, `ErrValueTooLong` | Shared sentinels, re-exported by the applicable storages so `errors.Is` matches across them            |
