# sqldialect

```go
import "github.com/altessa-s/go-atlas/internal/sqldialect"
```

SQL execution and text helpers shared by the `database/sql` storages — such as [outbox `sqldb`](../../reliability/outbox/storages/sqldb): table-name
validation and quoting, index naming, placeholder binding, serialized PostgreSQL schema creation, the transaction-aware `Pool`, and safe driver error
rendering. Internal to the module.

## Key types

| Symbol                                                            | Description                                                                                                                   |
|-------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `Style`                                                           | Identifier quote and placeholder form of a SQL flavor: `Postgres` (`"`, `$n`), `MySQL` (`?`)                                  |
| `Style.Table`                                                     | Validates each part with [sqlident](../sqlident) and quotes `name` or `schema.name`                                           |
| `Style.IndexName`                                                 | `<table>_<suffix>_idx`, or a readable prefix plus an FNV-32a hash when over 63 (long suffixes included in the hash)           |
| `Style.Bind`                                                      | Rewrites `?` as `$n` from a start index on numbered styles; positional styles pass through                                    |
| `Style.WrapPool`, `Pool`, `Pool.WithResolver`, `Scope`            | Wraps `*sql.DB` with per-call transaction resolution (`Tx`, `BeginScope`) and one retry on a pre-send PostgreSQL failure      |
| `SafeToRetry`, `StatementContext`, `DriverError`                  | Detects driver-confirmed pre-send failures and bounds `database/sql` connection replacement to one attempt per statement      |
| `SafeError`, `SQLState`, `SQLStates`                              | Renders `<op> failed (<type>, SQLSTATE <code>)` without raw driver or DSN text; preserves standard cancellation/sql sentinels |
| `ExecPostgresDDL`                                                 | Runs DDL in one transaction under sorted `pg_advisory_xact_lock`s on the table names                                          |
| `MySQLErrorNumber`                                                | Reads the first server error number through wrapped and joined trees without importing a SQL driver                           |
| `IsDuplicateKey`                                                  | Recognizes PostgreSQL SQLSTATE 23505 and MySQL error number 1062 in error trees                                               |
| `QueryKeys`, `InList`                                             | Read one batch of binary keys; expand a `%s` into an IN list of `n` placeholders                                              |
| `Unqualified`                                                     | Table part of a possibly qualified name                                                                                       |
| `ErrUnsupportedDialect`, `ErrValueTooLong`                        | Storage sentinels, shared alongside `sqlident.ErrInvalidTableName` so `errors.Is` matches across storages                     |
