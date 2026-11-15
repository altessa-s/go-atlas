# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/outbox"
```

Package `factory` provides a fluent builder for creating an `outbox.Outbox` from configuration. `OutboxBuilder` uses deferred error accumulation —
errors from any step are collected and returned at build time.

## Quick Start

```go
outbox, err := factory.New(cfg.Outbox).
    UseLogger(logger).
    UseScheduler(scheduler).
    BuildWithStorage(ctx, outboxmongofactory.Builder(db), handler)
```

The builder builds only the SQL storage. The MongoDB storage function comes from [`factory/reliability/outbox/storages/mongo`](storages/mongo)
(`Builder(db)`, `CollectionBuilder(col)`).

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

### Terminal

| Method                                      | Description                                                                                                                                                                                                                      |
|---------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `BuildWithStorage(ctx, build, handler)`     | Creates an outbox over the storage a `StorageFunc` creates (it receives the builder's logger), e.g. `outboxmongofactory.Builder(db)`; a nil function fails                                                                                                          |
| `BuildWithSQLDB(ctx, db, dialect, handler)` | Creates a SQL-backed outbox (PostgreSQL, MySQL or MariaDB) using a `*sql.DB`; creates the events table only with `ensureSchema: true` (otherwise it must exist); Save must run on the business transaction via `sqldb.WithTx` |

## Outbox startup

The `ctx` passed to a `BuildWith*` method bounds the startup I/O (schema creation, task registration). Outbox builders call `RegisterTasks` before
returning. A scheduler registration failure returns both a non-nil outbox and an error. Retain that instance to retry `RegisterTasks(ctx)`; callbacks
already installed remain inactive until all configured tasks are registered. Do not start serving traffic while registration is incomplete.
