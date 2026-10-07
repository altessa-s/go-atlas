# sqldb

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/sqldb"
```

SQL provider for [dlock](../../README.md) on PostgreSQL 12+ and MySQL 8.0+ / MariaDB 10.6+ through `database/sql`. Each lock key is one lease row,
taken by a conditional write, renewed in the background and released when the lock's context ends. Lease expiry is judged by the database clock,
and every acquisition increments a fencing token. The caller owns the `*sql.DB` and registers the driver.

## Constructors

| Function                     | Description                                                                         |
|------------------------------|-------------------------------------------------------------------------------------|
| `New(db, dialect, opts...)`  | Locker over `db` for `DialectPostgres` or `DialectMySQL`; performs no I/O           |
| `Locker.EnsureSchema(ctx)`   | Creates the table if absent; idempotent and safe to run concurrently                |

## Options

| Option                  | Default  | Description                                                                    |
|-------------------------|----------|--------------------------------------------------------------------------------|
| `WithTableName`         | `dlocks` | Table holding one lease row per key, optionally schema-qualified               |
| `WithTTL`               | 10s      | Lease length, truncated to whole milliseconds                                  |
| `WithRenewRatio`        | 1/3      | Fraction of the TTL after which the lease is renewed; TTL × ratio ≥ 1ms, < TTL |
| `WithOperationsTimeout` | 5s       | Bound of one acquisition attempt, renewal or release                           |
| `WithLogger`            | discard  | Logger for renewal and release failures                                        |

## Semantics

| Aspect          | Behavior                                                                                                              |
|-----------------|-----------------------------------------------------------------------------------------------------------------------|
| Acquire         | PostgreSQL: one `INSERT … ON CONFLICT DO UPDATE … WHERE expires_at <= now RETURNING fencing`. MySQL: the row is created on first use, then a conditional `UPDATE` and a token read commit together |
| Clock           | The database clock decides expiry: `clock_timestamp()` (not the transaction start) on PostgreSQL, `UTC_TIMESTAMP(6)` on MySQL |
| Fencing token   | +1 per acquisition of the key; rows are kept after release, so tokens never restart                                  |
| Renewal         | Every TTL × renew ratio; locks the row, then requires owner, token and an unexpired lease on a fresh clock read      |
| Late acquire    | A reply arriving after the TTL counted from the request is released and reported as `ErrLockNotHeld`                 |
| Ambiguous error | An acquisition that errored (including a failed commit) may still have applied: it is released by its unique owner id |
| Release         | Matches holder and fencing token: a stale holder cannot release the new holder's lease                               |
| Close           | Stops all renewals, waits for acquisitions in flight, releases every held lock; a repeated Close retries             |
| Transactions    | The locker uses the `*sql.DB` directly and never joins a caller's transaction                                        |

The table keeps one small row per key ever locked, and that row is the key's fencing history: deleting it, or restoring the table from a backup,
restarts or rewinds the key's tokens, so a stale holder could carry a higher token than the new one. Keep the rows of every key that may be locked
again.

## See also

- [providers/providertest](../providertest) — the contract suite this provider passes.
- [providers/mongo](../mongo) — the MongoDB provider it mirrors.
