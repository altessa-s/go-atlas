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

Build verifies every pool, calls the application hook on the direct primary pool, then registers each pool as `postgres.<name>`. The optional hook runs
with the earlier of the caller’s deadline and `postgres.migrationTimeout` (default `30s`, minimum `1ms`); for goose migrations, `WithLockTimeout`
limits lock acquisition within that overall budget. Build returns schema validation errors unchanged. A startup hook on a transaction-pooled primary
returns an error matching `corefactory.ErrUnsupported`. A startup failure closes already-opened pools.

The builder resolves the primary and reserved pools' TLS material before opening any connection. A named pool inherits the primary TLS block or
replaces it with its own. `UseTlsConfig(*tls.Config)` supplies a verified configuration for all pools and takes precedence over both blocks.
An injected config with `InsecureSkipVerify` or a TLS block the builder cannot load returns an error matching `coreerrs.ErrInvalidArgument`.
Without either source, pgx follows the DSN and environment. Its default `sslmode=prefer` allows unverified TLS and a plaintext retry.

On a SQL subsystem factory, call `UseSQLPool(pools.Binding("worker"))` to use the reserved pool for standalone calls and join a transaction
started on any pool in this group. Alternatively, use `UseSQLDB` with the outbox SQL store’s per-save `WithTx` helper. Configure one group per logical
database. [Runtime](../../../store/postgres) · [Configuration](../../../config/postgres)
