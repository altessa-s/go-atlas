# PostgreSQL pool factory

```go
pools, err := postgresfactory.New(&cfg).
    UseLogger(logger).
    UseMeter(meter).
    UseTracer(tracer).
    UseHealthCoordinator(coordinator).
    UseMigrator(func(ctx context.Context, db *sql.DB) error { return migrations.Run(ctx, db) }).
    Build(ctx)
if err != nil {
    return err
}
defer pools.Close()
db := pools[postgres.DefaultPool]
```

Build verifies every pool, calls the application hook on the direct primary pool, then registers each pool as `postgres.<name>`. The hook inherits the
caller’s deadline with a maximum of 30 seconds. A transaction-pooled primary rejects the hook during validation. A startup failure closes already-opened
pools. On a SQL subsystem factory, call `UseSQLPool(pools.Binding("worker"))` to use the reserved pool for standalone calls and join a transaction
started on any pool in this group. Alternatively, use `UseSQLDB` with the outbox SQL store’s per-save `WithTx` helper. Configure one group per logical
database. [Runtime](../../../store/postgres) · [Configuration](../../../config/postgres)
