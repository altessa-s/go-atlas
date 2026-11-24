# PostgreSQL pool factory

```go
pools, err := postgresfactory.New(&cfg).
    UseLogger(logger).
    UseMeter(meter).
    UseTracer(tracer).
    UseHealthCoordinator(coordinator).
    UseMigrations(migrations, "catalog").
    Build(ctx)
if err != nil {
    return err
}
defer pools.Close()
db := pools[postgres.DefaultPool]
```

Build verifies every pool, runs enabled migrations using the primary connection and registers each successful pool as `postgres.<name>`.
A startup failure closes already-opened pools. On a SQL subsystem factory, call `UseSQLPool(pools.Binding("worker"))` to use the reserved pool
for standalone calls and join a transaction started on any pool in this group. Configure one group per logical database. [Runtime](../../../store/postgres) · [Configuration](../../../config/postgres)
