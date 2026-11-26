# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/outbox"
```

Package `factory` provides a fluent builder for creating an `outbox.Outbox` from configuration. `OutboxBuilder` uses deferred error accumulation —
errors from any step are collected and returned at build time.

## Quick Start

Initialize the configuration with [`outboxconfig.Default()`](../../../config/outbox) or load the broker's `outbox` block.

```go
cfg := outboxconfig.Default()
ob, err := factory.New(&cfg).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseStorage(outboxconfig.StorageTypeMongo, outboxmongofactory.Builder(db)).
    Build(ctx, handler)

cfg.Storage = &outboxconfig.Storage{Type: outboxconfig.StorageTypeSQL}
cfg.Watch = outboxconfig.WatchNotify
ob, err = factory.New(&cfg).
    UseScheduler(scheduler).
    UseSQLDB(db).
    UseSQLListener(listener). // an outboxsql.Listener, e.g. over pgx
    Build(ctx, handler)
```

The storage is selected by `outbox.storage.type`. The builder builds the sqldb store itself; the MongoDB storage function comes from
[`factory/reliability/outbox/storages/mongo`](storages/mongo) (`Builder(db)`, `CollectionBuilder(col)`), so this package does not import the MongoDB
driver. A configured type without its dependency fails at `Build` with `factory/core.ErrDependencyRequired` (`UseSQLDB`, `UseSQLListener` for
`watch: notify`) or `ErrNotRegistered` (`UseStorage` for mongo).

Register a custom backend name with `UseStorage`. Before standalone validation or config loading, call
`cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())`.

## Methods

### Constructor

| Method     | Description                                            |
|------------|--------------------------------------------------------|
| `New(cfg)` | Creates an `OutboxBuilder` for the given outbox config |

### Dependencies

| Method                      | Description                                                                                |
|-----------------------------|--------------------------------------------------------------------------------------------|
| `UseLogger`                 | Sets the logger for the builder and all created components                                 |
| `UseDefaultLogger`          | Sets the logger to `slog.Default()`                                                        |
| `UseScheduler`              | Sets the task scheduler for background dispatch, unlock, expire, cleanup, and stats cycles |
| `UseOutboxOptions(opts...)` | Adds `outbox.Option` values after config, e.g. `outbox.WithShouldRetry`                    |
| `UseStorage(typ, build)`    | Registers a storage builder (`mongo`); nil functions fail                                  |
| `UseSQLDB(db)`              | Injects the `*sql.DB` of the `sqldb` storage; the caller owns it and registers the driver  |
| `UseSQLPool(db, resolve)`   | Supplies the pool and resolves the caller's transaction from each operation's context |
| `UseSQLListener(l)`         | Injects the `outboxsql.Listener` the `sqldb` store watches through with `watch: notify`    |
| `UseShutdownHooks(g)`       | Sets the watcher shutdown scope; defaults to the process-wide registry                     |

### Terminal

| Method                | Description                                                                                          |
|-----------------------|------------------------------------------------------------------------------------------------------|
| `Build(ctx, handler)` | Validates config, builds the store and outbox, and registers tasks; returns `nil, nil` when disabled |

The sqldb store creates the events table only with `storage.sqldb.ensureSchema: true` (otherwise it must exist). `Save` must run on the business
transaction via an injected resolver or `outboxsql.WithTx`. Set `storage.sqldb.keyOrder` on all dispatchers for per-key delivery.
Enable `storage.sqldb.asynchronousCommit` to allow PostgreSQL to acknowledge bookkeeping before flushing its WAL.

## Watch

Custom push storages must implement `outbox.Watcher`. Their storage function receives the selected watch mode and can reject modes it cannot serve.

With `watch: changeStream` (mongo) or `watch: notify` (sqldb, postgres dialect), a nonempty dispatch schedule and `UseScheduler` are required. `Build`
starts the watcher after all tasks register. The caller can stop it early with `ob.StopWatch()`. The watcher runs on a
context detached from the build context; a shutdown hook stops it and waits for it to return. `notify` also makes each SQL insert signal in the same
statement. The channel is derived from the table unless `storage.sqldb.notifyChannel` overrides it. Custom names are case-sensitive; quote mixed-case
names in psql `LISTEN`. A store that cannot serve the watch — a standalone mongod — is logged at warn level and dispatch stays on the schedule. Polling
remains necessary for retry backoffs and unlock sweeps.

## Outbox startup

The `ctx` passed to `Build` bounds the startup I/O (schema creation, task registration). The builder calls `RegisterTasks` before returning. A scheduler
registration failure returns a non-nil outbox and an error. Retry `RegisterTasks(ctx)` on that instance. After a successful retry, start the watcher
with `ob.StartWatch(lifetimeCtx)` and arrange to call `ob.StopWatch()`. Callbacks already installed remain inactive until all tasks are registered.
