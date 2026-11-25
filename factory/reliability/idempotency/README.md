# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
```

Package `factory` provides a fluent builder for creating an idempotency keeper from configuration. `KeeperBuilder` uses deferred error accumulation —
errors from any step are collected and returned at `Build(ctx)` time.

## Quick Start

```go
keeper, err := factory.New(cfg.Idempotency).
    UseLogger(logger).
    UseStorage(idempotencyconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

Register a custom backend name with `UseStorage`. Before standalone validation or config loading, call
`cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())`.

## Supported Storage Types

The builder builds `memory` and `sqldb` storage. `redis`, `nats` and `mongo` use the function registered with `UseStorage`; without one `Build` fails
naming the package to add. An unregistered custom type fails with `ErrUnsupported`.

| Type     | Backend                                            | Requires                                                                                                             |
|------------|------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `memory` | In-process map with TTL                            | `UseScheduler` (optional)                                                                                            |
| `redis`  | Redis                                              | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/reliability/idempotency/storages/redis`](storages/redis) |
| `nats`   | NATS JetStream KV                                  | `UseStorage("nats", natsfactory.Builder(js))` from [`factory/reliability/idempotency/storages/nats`](storages/nats)         |
| `sqldb`  | PostgreSQL, MySQL or MariaDB, joins transactions     | `UseSQLDB(db)`                                                                                                        |
| `mongo`  | MongoDB collection, joins the caller's transaction | `UseStorage("mongo", mongofactory.Builder(db))` from [`factory/reliability/idempotency/storages/mongo`](storages/mongo)     |

SQL uses `UseSQLDB(db)` with `storage.type: sqldb` and `storage.sqldb: {dialect, table, ensureSchema, purgeInterval, purgeBatchSize}`. A missing
database returns `factory/core.ErrDependencyRequired`. With `ensureSchema: true`, `Build(ctx)` creates the table and index within the startup context,
capped at 30 seconds. Otherwise it performs no startup SQL I/O. The caller owns the driver and database lifetime. The backend applies `Config.TTL` to
locks and results. See the [SQL storage](../../../reliability/idempotency/storages/sqldb/README.md) for `WithTx`, transaction restart errors and
expired-row cleanup.

```go
keeper, err := factory.New(cfg.Idempotency).UseSQLDB(db).Build(ctx)
```

The SQL cleanup interval defaults to one minute through configuration; zero disables it. Cleanup starts lazily on the first key write. The builder owns
the SQL storage and registers its `Close(ctx)` with `UseShutdownHooks(group)`, or the process-wide `go-atlas-core/runtime` shutdown registry when no
group is supplied. Run those hooks before closing the injected database. A dedicated group lets callers stop and rebuild the subsystem independently.

## Methods

### Constructor

| Method | Description |
|------------|------------------------------------------------------------|
| `New(cfg)` | Creates a `KeeperBuilder` for the given idempotency config |

### Dependencies

| Method | Description |
|----------------------|-------------------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                                          |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                                 |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`, `nats`, `mongo`) |
| `UseScheduler`     | Sets the scheduler for background cleanup task registration                                         |
| `UseShutdownHooks` | Sets the shutdown hook group for the SQL storage (default: process-wide runtime registry)           |
| `UseSQLDB`         | Injects the database for built-in SQL storage                                                       |
| `UseSQLPool`       | Supplies the pool and per-call transaction resolver for built-in SQL storage                        |
| `UseMeter`         | Sets the metrics collector passed to the keeper                                                     |

### Caller binding

| Method | Description |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `UseOwner` | Sets the `idempotency.OwnerFunc` labeling each lock with its caller (default: [`reliability/idempotency/principalowner.PrincipalOwner`](../../../reliability/idempotency/principalowner)) |
| `UseKeyNamespace` | Sets the `idempotency.KeyNamespaceFunc` isolating keys per context (default: the empty namespace) |
| `UseReplayAuthorizer` | Sets the `idempotency.ReplayAuthorizer` deciding every replay from the recorded owner label |

The keeper binds every record to the verified principal of the request (tenant and subject from `auth/principal` via
`reliability/idempotency/principalowner`): another caller presenting the key is refused the record (`idempotency.ErrOwnerMismatch`). A request without a
principal records no owner, and the HTTP middleware and gRPC interceptor never replay such a record or echo its entity ID — they answer "already used".
Pass `UseOwner` to bind records to another identity.

### Terminal

| Method | Description |
|----------|----------------------------------------------|
| `Build` | Assembles and returns the idempotency keeper |

SQL configuration maps purgeInterval and purgeBatchSize to the store options (defaults "1m" and 1000). A zero interval disables background purging. The
builder registers Close with UseShutdownHooks(group) or the process-wide runtime shutdown registry; run shutdown hooks before closing the injected
database.
