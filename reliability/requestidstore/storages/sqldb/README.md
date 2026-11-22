# sqldb

```go
import "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/sqldb"
```

PostgreSQL, MySQL and MariaDB storage for AIP-155 request IDs. Pass the storage to the go-aip guard described in [requestidstore](../../README.md).

## Usage

Call `New(db, dialect, opts...)` with an open connection pool. Call `EnsureSchema(ctx)` at startup or install equivalent DDL through migrations. The
caller owns the pool and should call `Close(ctx)` on the storage before closing the pool.

## Atomic operations

| Operation | Database behavior |
|-----------|-------------------|
| Reserve   | The primary key serializes competing inserts. After a duplicate, read the live entry or replace an expired row with a conditional update. Retry if a concurrent release or purge removed the row. |
| Complete  | Update the row only when the state is pending, the token matches byte for byte and the caller's time precedes expiry. The update writes the result and its new expiry together. |
| Release   | Delete with the same state, token and expiry predicate. |

Expiry is stored as Unix milliseconds. The background collector removes rows in indexed batches. A row is eligible once the latest observed caller time reaches
its expiry plus the cleanup grace.
Set the grace above the maximum skew between callers sharing the table.
In an idle deployment, expired rows can remain until calls resume. Logical expiry is checked on every operation. Automatic collection starts on first
use. Call `PurgeExpired(ctx)` to drain manually.

## Options

| Option               | Default              | Purpose                                        |
|----------------------|----------------------|------------------------------------------------|
| `WithCleanupGrace`   | `1h`                 | Extra retention for clock skew (minimum 1 ms). |
| `WithTableName`      | `request_id_entries` | Select a table, optionally schema-qualified.   |
| `WithPurgeInterval`  | One minute           | Set zero to disable periodic cleanup.          |
| `WithPurgeBatchSize` | 1000                 | Bound the rows removed per statement.          |
| `WithLogger`         | Discard              | Report background collection errors.           |
