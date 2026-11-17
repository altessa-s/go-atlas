# sqldb

```go
import "github.com/altessa-s/go-atlas/coord/dlock/providers/sqldb"
```

SQL provider for [dlock](../../README.md) on PostgreSQL 12+ and MySQL 8.0+ / MariaDB 10.6+ through `database/sql`. Each lock key is one lease row, taken
by a conditional write, renewed in the background and released when the lock's context ends. Lease expiry is judged by the database clock, and every
acquisition increments a fencing token. The caller owns the `*sql.DB` and registers the driver.

## Constructors

| Function                        | Description                                                                   |
|---------------------------------|-------------------------------------------------------------------------------|
| `New(db, dialect, opts...)`     | Locker over `db` for `DialectPostgres` or `DialectMySQL`; performs no I/O     |
| `Locker.EnsureSchema(ctx)`      | Creates the table if absent; idempotent and safe to run concurrently          |
| `Locker.PurgeExpired(ctx)`      | One `WithExpiredCleanup` sweep now; returns the rows deleted                  |

## Options

| Option                      | Default      | Description                                                                                              |
|-----------------------------|--------------|----------------------------------------------------------------------------------------------------------|
| `WithTableName`             | `dlocks`     | Table holding one lease row per key, optionally schema-qualified                                         |
| `WithTTL`                   | 10s          | Lease length, truncated to whole milliseconds                                                            |
| `WithRenewRatio`            | 1/3          | Fraction of the TTL after which the lease is renewed; TTL × ratio ≥ 1ms, < TTL                           |
| `WithOperationsTimeout`     | 5s           | Bound of one acquisition attempt, renewal, release or cleanup batch                                      |
| `WithoutRenewal`            | off          | Hard leases: never renewed, a lease ends one TTL after its acquisition request; the ratio is ignored     |
| `WithExpiredCleanup`        | off          | Background sweep deleting rows `grace` after their lease ended, by the database clock                    |
| `WithPurgeInterval`         | 1 minute     | Time between purge passes; zero disables the loop                                                        |
| `WithPurgeBatchSize`        | 1000         | Maximum rows per purge statement                                                                         |
| `WithLogger`                | discard      | Logger for renewal, release and cleanup failures                                                         |

## Semantics

| Aspect          | Behavior                                                                                                                                                                                           |
|-------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Acquire         | PostgreSQL: one `INSERT … ON CONFLICT DO UPDATE … WHERE expires_at <= now RETURNING fencing`. MySQL: the row is created on first use, then a conditional `UPDATE` and a token read commit together |
| Clock           | The database clock decides expiry: `clock_timestamp()` (not the transaction start) on PostgreSQL, `UTC_TIMESTAMP(6)` on MySQL                                                                      |
| Fencing token   | +1 per acquisition of the key; rows are kept after release, so tokens never restart (unless `WithExpiredCleanup` deletes them)                                                                     |
| Renewal         | Every TTL × renew ratio; locks the row, then requires owner, token and an unexpired lease on a fresh clock read                                                                                    |
| Hard lease      | `WithoutRenewal`: no renewal; the lock counts as ended one TTL after the request was sent (the database's lease starts later, so it never outlives it), and releasing it then is a no-op      |
| Late acquire    | A reply arriving after the TTL counted from the request is released and reported as `ErrLockNotHeld`                                                                                               |
| Ambiguous error | An acquisition that errored (including a failed commit) may still have applied: it is released by its unique owner id                                                                              |
| Release         | Matches holder and fencing token: a stale holder cannot release the new holder's lease                                                                                                             |
| Close           | Stops all renewals, waits for acquisitions in flight, releases every held lock; a repeated Close retries                                                                                           |
| Transactions    | The locker uses the `*sql.DB` directly and never joins a caller's transaction                                                                                                                      |
| Indexes         | `<table>_expires_idx` on `(expires_at)` supports cleanup; MySQL reads keys without locking and deletes through the primary key                                                                                |

The table keeps one small row per key ever locked, and that row is the key's fencing history: deleting it, or restoring the table from a backup,
restarts or rewinds the key's tokens, so a stale holder could carry a higher token than the new one. Keep the rows of every key that may be locked
again.

## Expired cleanup and hard leases

For many short-lived keys (one per work item), `WithExpiredCleanup(grace)` bounds the table: from the first `Lock` until `Close`, the locker sweeps at
`WithPurgeInterval` (one minute by default) and deletes every row whose lease expired or was released more than `grace` ago by the database clock, in
batches of at most `WithPurgeBatchSize` rows (default 1000). On PostgreSQL each batch is one `DELETE … WHERE lock_key IN (SELECT … FOR UPDATE SKIP
LOCKED)`; on MySQL the batch's keys are read without locking and deleted through the primary key with the condition repeated, so a sweep never locks a
live lease's row. Every replica sweeps; concurrent sweeps are harmless. `PurgeExpired` runs one sweep on demand, for example from a scheduler, and does
nothing without the option. A MySQL acquisition whose row a sweep deleted between creating and taking it starts over.

A deleted key starts again at fencing token 1, so tokens are monotonic only while a key is locked again within `grace` of its last lease: pick a grace
longer than any holder may act on a token, or leave cleanup off when tokens fence external writes. A row may outlive `grace` by up to one sweep
interval. `WithoutRenewal` suits work that must not outlive its lease: bound the work with a deadline of one TTL taken before `Lock`, which ends no
later than the database's lease.

`WithPurgeInterval(0)` disables automatic sweeps while `PurgeExpired(ctx) (int64, error)` remains available. It returns the total across all completed
batches, including batches before an error. Close permanently stops the loop and waits within the context; a later Close waits again if a prior wait
timed out. Loop failures and recovered panics go to `WithLogger` (discard by default).

## See also

- [providers/providertest](../providertest) — the contract suite this provider passes.
- [providers/mongo](../mongo) — the MongoDB provider it mirrors.

`EnsureSchema` adds the `(expires_at)` index to existing tables as well as new ones: PostgreSQL uses `CREATE INDEX IF NOT EXISTS` under its DDL advisory
lock; MySQL/MariaDB reconciles missing indexes by name after table creation. Each purge statement is bounded by `WithOperationsTimeout`. Batch limits
are bound parameters. Background passes use a fresh context without request values or transactions.
