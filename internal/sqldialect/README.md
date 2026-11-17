# sqldialect

```go
import "github.com/altessa-s/go-atlas/internal/sqldialect"
```

SQL text helpers shared by the `database/sql` storages — such as [outbox `sqldb`](../../reliability/outbox/storages/sqldb): table-name validation
and quoting, index naming, placeholder binding, serialized PostgreSQL schema creation, and MySQL index reconciliation. Internal to the module.

## Key types

| Symbol                                         | Description                                                                                  |
|------------------------------------------------|----------------------------------------------------------------------------------------------|
| `Style`                                        | Identifier quote and placeholder form of a SQL flavor: `Postgres` (`"`, `$n`), `MySQL` (`?`) |
| `Style.Table`                                  | Validates `name` or `schema.name` (each part at most `MaxIdentLen` = 63) and quotes it       |
| `Style.IndexName`                              | `<table>_<suffix>_idx`, or a readable prefix plus an FNV-32a hash when over 63 (long suffixes included in the hash)  |
| `Style.Bind`                                   | Rewrites `?` as `$n` from a start index on numbered styles; positional styles pass through   |
| `ExecPostgresDDL`                              | Runs DDL in one transaction under sorted `pg_advisory_xact_lock`s on the table names         |
| `MySQLIndex`                                  | Secondary index name (plain or backtick-quoted) and trusted SQL column list                  |
| `AddMissingMySQLIndexes`                       | Adds absent secondary indexes to existing MySQL/MariaDB tables; tolerates concurrent creation |
| `MySQLErrorNumber`                            | Reads the server error number through wrappers without importing a SQL driver              |
| `Unqualified`, `Qualifier`                     | Table and schema parts of a possibly qualified name                                          |
| `ErrUnsupportedDialect`, `ErrInvalidTableName` | Shared sentinels, re-exported by both storages so `errors.Is` matches across them            |

## MySQL index reconciliation

Call `AddMissingMySQLIndexes` after creating the table. Pass unquoted schema and table identifiers; an empty schema selects the connection's
default database. Index names may come directly from `MySQL.IndexName`. Column lists are trusted SQL fragments authored by the storage.
Reconciliation checks names only: it leaves existing index definitions unchanged and ignores error 1061 when another instance adds an index
first. Other catalog and DDL errors are returned. No SQL driver is imported.

```go
err := sqldialect.AddMissingMySQLIndexes(ctx, db, "app", "events", []sqldialect.MySQLIndex{
    {Name: sqldialect.MySQL.IndexName("events", "created"), Columns: "status, created_at"},
})
```
