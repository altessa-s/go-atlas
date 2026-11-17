# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/outbox"
```

Package `factory` provides a fluent builder for creating an `outbox.Outbox` from configuration. `OutboxBuilder` uses deferred error accumulation —
errors from any step are collected and returned at build time.

## Quick Start

```go
// outbox.storage.type: mongo
ob, err := factory.New(&cfg.Outbox).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseStorage(brokerconfig.OutboxStorageTypeMongo, outboxmongofactory.Builder(db)).
    Build(ctx, handler)

// outbox.storage.type: sqldb, outbox.watch: notify
ob, err := factory.New(&cfg.Outbox).
    UseScheduler(scheduler).
    UseSQLDB(db).
    UseSQLListener(listener). // an outboxsql.Listener, e.g. over pgx
    Build(ctx, handler)
```

The storage is selected by `outbox.storage.type`. The builder builds the sqldb store itself; the MongoDB storage function comes from
[`factory/reliability/outbox/storages/mongo`](storages/mongo) (`Builder(db)`, `CollectionBuilder(col)`), so this package does not import the MongoDB
driver. A configured type without its dependency fails at `Build` with `factory/core.ErrDependencyRequired` (`UseSQLDB`, `UseSQLListener` for
`watch: notify`) or `ErrNotRegistered` (`UseStorage` for mongo).

## Methods

### Constructor

| Method     | Description                                            |
|------------|--------------------------------------------------------|
| `New(cfg)` | Creates an `OutboxBuilder` for the given outbox config |

### Dependencies

| Method                      | Description                                                                                                                     |
|-----------------------------|---------------------------------------------------------------------------------------------------------------------------------|
| `UseLogger`                 | Sets the logger for the builder and all created components                                                                      |
| `UseDefaultLogger`          | Sets the logger to `slog.Default()`                                                                                             |
| `UseScheduler`              | Sets the task scheduler for background dispatch, unlock, expire, cleanup, and stats cycles                                      |
| `UseOutboxOptions(opts...)` | Adds code-supplied `outbox.Option` values applied after the configuration (they win on overlap), e.g. `outbox.WithShouldRetry` |
| `UseStorage(typ, build)`    | Registers the `StorageFunc` of a storage type (`mongo`); it receives the config and the builder's logger; a nil function fails  |
| `UseSQLDB(db)`              | Injects the `*sql.DB` of the `sqldb` storage; the caller owns it and registers the driver                                      |
| `UseSQLListener(l)`         | Injects the `outboxsql.Listener` the `sqldb` store watches through with `watch: notify`                                        |
| `UseShutdownHooks(g)`       | Scope the started watcher is stopped by; defaults to the process-wide registry (`go-atlas-core/runtime.OnShutdown`)            |

### Terminal

| Method                | Description                                                                                                                         |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `Build(ctx, handler)` | Validates `storage` and `watch`, builds the configured store, creates the outbox and registers its tasks; `nil, nil` when disabled |

The sqldb store creates the events table only with `storage.sqldb.ensureSchema: true` (otherwise it must exist). `Save` must run on the business
transaction via `outboxsql.WithTx`.

## Watch

With `watch: changeStream` (mongo) or `watch: notify` (sqldb, postgres dialect) `Build` starts `outbox.Outbox.Watch` in its own goroutine once the
tasks are registered, on a context detached from the build context, and registers a shutdown hook that stops it and waits for it to return. `notify`
also makes the sqldb store signal the channel derived from its table in every saving transaction (`outboxsql.WithNotify`). A store that cannot serve
the watch — a standalone mongod — is logged at warn level and dispatch stays on the schedule, which runs in every mode.

## Outbox startup

The `ctx` passed to `Build` bounds the startup I/O (schema creation, task registration). The builder calls `RegisterTasks` before returning. A scheduler
registration failure returns both a non-nil outbox and an error, and no watcher is started. Retain that instance to retry `RegisterTasks(ctx)`;
callbacks already installed remain inactive until all configured tasks are registered. Do not start serving traffic while registration is incomplete.
