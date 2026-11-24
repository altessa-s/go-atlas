# postgres

```go
import "github.com/altessa-s/go-atlas/store/postgres"
```

PostgreSQL connection pools, transactions and forward migrations over pgx and `database/sql`.

## Opening a pool

```go
db, err := postgres.Open(ctx, dsn,
    postgres.WithPooling(postgres.PoolingTransaction),
    postgres.WithMaxOpenConnections(16),
    postgres.WithMaxIdleConnections(8),
    postgres.WithMeter(meter),
    postgres.WithTracer(tracer),
    postgres.WithLogger(logger),
)
if err != nil {
    return err
}
defer db.Close()
```

`dsn` is a `redacted.RedactedString`. The returned `*sql.DB` owns the connections and the pool metric observer. Use `Checker(db)` for health checks.
The configuration factory opens several named pools, each with its own connection reservation and optional DSN. The primary pool is `DefaultPool`.
Pass a reserved pool to a repository directly or use `WithPool(ctx, primary, reserved)` to route one operation to a pool for the same database.

| Setting | Default | Effect |
| --- | --- | --- |
| Pooling | direct | Session lifetime or transaction lifetime |
| Max open / idle connections | 64 / 64 | Per-pool limits. Zero idle closes connections on return |
| Connection max lifetime / idle time | 30m / 5m | Connection recycling |
| Connect timeout | 5s | Connection establishment |
| Statement / lock / idle transaction timeouts | 10s / 3s / 30s | PostgreSQL execution limits |
| Pool name | foreground | Fixed label in pool metrics |

In direct mode the connector sets timeout defaults at startup. In both modes an explicit transaction receives its limits with `BEGIN` and
`SET LOCAL` in a single round trip. `WithTransactionSettings` can override the statement, lock and idle-in-transaction timeouts, force a custom
plan, or set `synchronous_commit`. `BeginAsyncCommit` opens an independent transaction with asynchronous WAL flush. A server crash can lose a
recently acknowledged asynchronous commit.

For transaction pooling, configure PgBouncer to track protocol-level prepared statements (`max_prepared_statements > 0`). Give the database role
positive `statement_timeout`, `lock_timeout` and `idle_in_transaction_session_timeout` defaults for autocommit work. `Open` checks these
settings before returning the pool. The connector retires a pooled connection following a failed prepare. The default execution mode uses pgx’s statement cache. Set `default_query_exec_mode=exec` in the DSN to use the extended protocol without caching.
Application SQL should use transaction-local
settings and transaction-scoped locks.

## Application transactions

```go
q := postgres.Querier(db)
return postgres.WithTransaction(ctx, db, func(ctx context.Context) error {
    if _, err := q.ExecContext(ctx, "INSERT INTO items (id) VALUES ($1)", id); err != nil {
        return err
    }
    return events.Save(ctx, event)
})
```

Use `UseSQLDB` for distributed locks. For stores that enlist, call `UseSQLPool(pools.Binding(name))` on the subsystem factory.
Only outbox saves enlist. Standalone operations use the named pool, and operations in a transaction use
the caller’s connection from any pool in the group. Configure one group per logical database. When constructing a store directly, pass its
`WithTxResolver(postgres.Tx)` option for stores that use that same logical database. With a plain `UseSQLDB` injection, attach the transaction to each outbox save with the outbox SQL store’s `WithTx` helper. A transaction belongs to one call. Its owner commits on
success and rolls back on error or panic. Nested `WithTransaction` and `WithSnapshot` join the outer transaction and inherit its isolation and
access mode. A top-level snapshot uses read-only repeatable read. Use `WithSavepoint` to roll back a nested operation while continuing the outer
transaction. A savepoint covers the entire transaction. Keep other goroutines off that transaction until the savepoint handler returns.

### Units of work

```go
committer, err := postgres.NewCommitter(db)
if err != nil {
    return err
}
runner := uow.New(committer, logger, uow.WithDetector(postgres.Transaction))
return runner.Run(ctx, func(txCtx context.Context) error {
    return repo.Save(txCtx, item)
})
```

The runner uses `postgres.Transaction` to recognize open write transactions across pools, including joins and savepoints. Use `committer.Detect`
to inspect only its selected pool. The transaction owner alone may call
`Commit` or `Rollback`. After completion or cancellation of the owner context, `Tx` returns the attached handle and `false`. SQL store resolvers
treat that handle as a closed binding. Statements and nested transactions on a retained scope fail with `sql.ErrTxDone`.

The default executes the body once. For a replay-safe body, `WithTxRetries(n)` permits up to n additional attempts after SQLSTATE 40001 or 40P01.
`WithTxRetryDelay` sets the initial positive backoff. `WithTxTimeout` caps the root transaction including backoff and retries. Each attempt gets
fresh registrations. A replay-enabled committer rejects a join with `ErrUnsafeReplay`; `BeginAsyncCommit` returns the same error inside its body.
Cross-pool nesting returns `ErrDifferentDatabase`. Use the supplied context and enlisted repositories throughout a replayable body. Independent writes
through a raw pool, detached roots and external effects are the caller's responsibility.

A server-rejected commit discards registrations and preserves the database error for inspection. An ambiguous commit yields `uow.ErrUnknownOutcome`.
A joined panic poisons the root even if the caller recovers. Use a savepoint to recover from a statement error before continuing the transaction.

Register a callback with `AfterCommit`, or use `AfterCommitContext` to retain request values with a fresh cancellation and transaction scope.
Outside a transaction these callbacks run immediately. `NotifyAfterCommit` coalesces notifications to a channel across a transaction.
Finish all operations and hook registrations before returning from the handler.

On pools opened with `Open` or `NewConnector`, `Querier` retries a standalone statement once if the driver establishes that it sent no data.
The subsystem SQL stores apply the same limit through their database/sql connection leases. A transaction stays on its
own connection, and an iteration or scan error is final. Use an executor with contexts for the same logical database.
Call `CheckDatabase(ctx, db)` at a boundary that accepts a transaction context and a separately supplied pool. Code using `*sql.DB` directly receives Go's native bad-connection retry policy.
`Classify` identifies integrity violations (with the constraint name), retryable transaction failures, connection failures, statement timeouts
and cancellations. Use `kind.RetryableTransaction()` to identify failures eligible for whole-transaction replay. An ambiguous connection failure can represent a
committed write. PostgreSQL reports both server-side timeout and cancellation as SQLSTATE 57014. Use English server messages for that distinction. An unrecognized 57014 diagnostic returns `Unknown`.

## Migrations

Give `Migrate(ctx, db, source, schema)` an `fs.FS` rooted at the migration directory, for example via `fs.Sub` on an embedded filesystem.
Use a schema name other than PostgreSQL’s search-path aliases `$user` and `pg_temp`.
Files use positive numeric versions and the suffix `_<name>.up.sql`. Each version must occur once. Only forward migration files belong in the
directory. The runner takes a schema-specific transaction-scoped advisory lock before creating the schema or its `schema_migrations` ledger.
It sets the local search path to the schema, applies pending versions in numeric order and commits the DDL and ledger entries together.
A context deadline supplies the migration lock and statement budgets. Without a deadline, the pool timeouts apply.
Migration files must contain transactional SQL. Use the same ledger schema when moving an existing deployment. If an older runner uses a
different lock key, finish its startup migrations before enabling the new runner on that database.

## Migrating an application

1. Replace the local PostgreSQL configuration with `config/postgres.Config`. Move each reservation into `pools` and carry over its size, lock
   timeout and optional DSN. Listing and count budgets remain available to the application's query layer.
2. Replace local startup with `factory/store/postgres.New(&cfg).UseMigrations(source, schema).Build(ctx)`. Set `migrations: true` to apply
   the embedded files. Inject the logger, meter, tracer and health registrar on the builder. Close the returned pools at shutdown.
3. Replace the local transaction, savepoint, snapshot, commit-hook and settings helpers with their same-named functions here. Pass
   `postgres.Querier(db)` to repositories constructed once, or `postgres.Queryer(ctx, db)` when constructing a query object per call.
   For example, a client that uses sqlc with `database/sql` can call `queries := generated.New(postgres.Querier(db))`.
4. Pass `pools.Binding(name)` to builders that support `UseSQLPool`, or inject `WithTxResolver(postgres.Tx)` directly.
   Remove per-call outbox transaction attachment.
   Use `Classify` or `UniqueViolation` to map constraints to application errors. Column encryption, domain error policy and listing logic stay
   with the application.

Pool metrics are listed in the [metrics reference](../../docs/metrics.md#postgres). Client spans require a sampled, recording parent.
They contain the statement name or operation, normalized SQL text (up to 2 KiB), database and server attributes, affected rows and SQLSTATE.
Bind parameter values and server error messages are omitted. Use placeholders for sensitive values because literals written into SQL are part of its text.
