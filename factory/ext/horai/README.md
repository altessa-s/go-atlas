# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/horai"
```

Package `factory` builds a [go-horai](https://github.com/altessa-s/go-horai) `scheduler.Scheduler` from configuration. `SchedulerBuilder` uses
deferred error accumulation — errors from any step are collected and returned at `Build(ctx)` time. The caller starts and stops the scheduler.

## Quick Start

```go
sched, err := factory.New(cfg.Scheduler).
    UseLogger(logger).
    UseStorage(schedulerconfig.StorageTypeMongo, mongofactory.Builder(db)).
    UseLeaderElector(elector).
    Build(ctx)
```

The builder maps every `schedulerconfig.Config` field to a scheduler option, including `instanceID` → `scheduler.WithInstanceID` (empty keeps the
per-process random ID).

Register a custom backend name with `UseStorage`. Before standalone validation or config loading, call
`cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())`.

## Supported Storage Backends

| Type     | Backend                                                | Requires                                                                                |
|----------|--------------------------------------------------------|-----------------------------------------------------------------------------------------|
| `memory` | In-process (default)                                   | —                                                                                       |
| `mongo`  | MongoDB                                                | `UseStorage("mongo", mongofactory.Builder(db))` from [`factory/ext/horai/mongo`](mongo)     |
| `redis`  | Redis                                                  | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/ext/horai/redis`](redis) |
| `sqldb`  | PostgreSQL / MySQL / MariaDB (`storage.sqldb.dialect`) | `UseSQLDB`                                                                              |

`historyStorage` moves execution history to a backend of its own (`scheduler.WithHistoryStore`); task state stays in `storage`.

| Type         | Backend                                                 | Requires                                                                                                 |
|--------------|---------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| `clickhouse` | ClickHouse table; a zero `ttl` takes `historyRetention` | `UseHistoryStorage("clickhouse", chfactory.Builder(conn))` from [`factory/ext/horai/clickhouse`](clickhouse) |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `SchedulerBuilder` for the given scheduler config |

### Dependencies

| Method              | Description                                                                                                                                                                                                                                                                             |
|---------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `UseLogger`         | Sets the logger for the builder and all created components                                                                                                                                                                                                                              |
| `UseDefaultLogger`  | Sets the logger to `slog.Default()`                                                                                                                                                                                                                                                     |
| `UseMeter`          | Sets the `metrics.Meter` for the `scheduler_*` metrics                                                                                                                                                                                                                                  |
| `UseReadinessProbe` | Sets the readiness probe that gates dispatch (`scheduler.WithReadiness`)                                                                                                                                                                                                                |
| `UseLeaderElector`  | Sets the leader elector for distributed scheduling                                                                                                                                                                                                                                      |
| `UseStorage`        | Registers the `StorageFunc` of a task storage type the builder does not build (`mongo`, `redis`); a configured `storage` type without one fails at `Build` naming the package to add                                                                                                    |
| `UseSQLDB`          | Sets the `*sql.DB` handle for SQL storage backends (set `storage.sqldb.ensureSchema: true` to create the schema during `Build`; otherwise create it first: `EnsureSchema` on a `sqldb.New` storage with the same handle, dialect and tables, or migrations)                             |
| `UseHistoryStorage` | Registers the `HistoryStorageBuilder` of a history storage type; a configured `historyStorage` type without one fails at `Build` naming the package to add                                                                                                                              |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the scheduler with storage created from config; `ctx` bounds the schema/index creation |

## Concurrency

`BuildLimitFunc` converts the [concurrency schema](../../../config/scheduler) into a limit function. A static limit of zero selects the scheduler
default.

## Page tokens

go-horai leaves cursor signing to the transport: the `paging` section is consumed by the gRPC handler
([`scheduler`](../../../transport/grpc/handlers/scheduler)), built with `pagetokenfactory.New(cfg.Paging).Build()` and passed to `scheduler.New`.
