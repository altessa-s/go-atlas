# sqldialect

```go
import "github.com/altessa-s/go-atlas/internal/sqldialect"
```

SQL text helpers shared by the `database/sql` storages — [scheduler `sqldb`](../../service/scheduler/storages/sqldb) and
[outbox `sqldb`](../../data/outbox/storages/sqldb): table-name validation and quoting, index naming, placeholder binding, and serialized
PostgreSQL schema creation. Internal to the module.

## Key types

| Symbol                                         | Description                                                                                  |
|------------------------------------------------|----------------------------------------------------------------------------------------------|
| `Style`                                        | Identifier quote and placeholder form of a SQL flavor: `Postgres` (`"`, `$n`), `MySQL` (`?`) |
| `Style.Table`                                  | Validates `name` or `schema.name` (each part at most `MaxIdentLen` = 63) and quotes it       |
| `Style.IndexName`                              | `<table>_<suffix>_idx`, or a readable prefix plus an FNV-32a hash of the table when over 63  |
| `Style.Bind`                                   | Rewrites `?` as `$n` from a start index on numbered styles; positional styles pass through   |
| `ExecPostgresDDL`                              | Runs DDL in one transaction under sorted `pg_advisory_xact_lock`s on the table names         |
| `Unqualified`, `Qualifier`                     | Table and schema parts of a possibly qualified name                                          |
| `ErrUnsupportedDialect`, `ErrInvalidTableName` | Shared sentinels, re-exported by both storages so `errors.Is` matches across them            |
