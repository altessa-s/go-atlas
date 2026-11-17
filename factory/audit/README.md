# factory

```go
import "github.com/altessa-s/go-atlas/factory/audit"
```

Package `factory` provides a fluent builder for creating an audit auditor from configuration. `AuditorBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build(ctx)` time.

## Quick Start

The builder creates the whole pipeline from configuration — `storage` picks the backend, `dispatch` tunes the engine in front of it:

```go
auditor, err := factory.New(cfg.Audit).
    UseLogger(logger).
    Build(ctx)
```

Supply a dispatcher only to override that:

```go
auditor, err := factory.New(cfg.Audit).
    UseLogger(logger).
    UseDispatcher(eng). // takes `storage` and `dispatch` out of play
    Build(ctx)
```

## Methods

### Constructor

| Method              | Description                                                                                                                   |
|--------------|----------------------------------------------------------|
| `New(cfg)` | Creates an `AuditorBuilder` for the given audit config |

### Dependencies

| Method              | Description                                                                                                                   |
|----------------------|-------------------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                                          |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                                 |
| `UseDispatcher`    | Sets the `audit.Dispatcher` (must already be started); overrides `storage` and `dispatch`           |
| `UseStorage`       | Registers the `StorageBuilder` of a storage type the builder does not build (`mongo`, `clickhouse`) |
| `UseSQLDB`         | Sets the `*sql.DB` for `storage.type: sqldb`; the caller registers the driver                       |
| `UseShutdownHooks` | Sets the `runtime.HookGroup` the builder registers what it owns into                                |

### Terminal

| Method                  | Description                                                                                                                       |
|-------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `Build(ctx)`            | Assembles, starts, and returns the audit auditor; `ErrDisabled` when `enabled` is false; `ctx` bounds the storage startup I/O     |
| `BuildStorage(ctx)`     | Returns the configured `audit.Storage` for reading events (e.g. with `audit.FetchPage`); SQL storage also gets a shutdown hook    |
| `BuildPageTokens`       | Returns `*audit.PageTokens` from `paging`, or nil when `paging` is not configured                                                 |

## Storage

| `storage.type` | Requires                                                                                                                 | Notes                                                                                                           |
|------------------|----------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| `memory`       | --                                                                                                                       | In-process, unbounded; events are lost on restart                                                               |
| `mongo`        | `UseStorage("mongo", auditmongofactory.Builder(db))` from [`factory/audit/storages/mongo`](storages/mongo)          | `storage.mongo` tunes collection name, index timeout and TTL; omitting the section means all defaults           |
| `clickhouse`   | `UseStorage("clickhouse", chfactory.Builder(conn))` from [`factory/audit/storages/clickhouse`](storages/clickhouse) | `storage.clickhouse` is required; `schemaMigrationMode` is applied, then the table schema is checked at startup |
| `sqldb`        | `UseSQLDB`                                                                                                               | `storage.sqldb` is required; `ensureSchema` creates the table at startup, otherwise the schema must exist       |

An `sqldb` type without a database fails with `ErrSQLDBRequired`, and a `mongo` or `clickhouse` type without a registered builder with an error naming
the package to add, rather than silently falling back to memory.

`storage.sqldb.ttl` maps to SQL storage `WithTTL`, and the builder's logger is passed to `WithLogger`. Positive TTL starts background retention every
minute in batches of up to 1,000 rows; zero keeps events forever. MongoDB's `storage.mongo.ttl` instead configures a server TTL index. `BuildStorage`
callers must close the returned storage to stop its retention loop; the injected database remains caller-owned.

## Paging

`paging.signingKey` (at least 32 bytes, the same on every replica), `paging.previousKeys` and `paging.tokenTTL` configure the page tokens
`BuildPageTokens` returns for `audit.FetchPage`. Writing events does not need them.

## Ownership and shutdown

When the builder creates the engine it also owns it, so it registers the engine's shutdown — an engine nobody stops keeps its workers and, with WAL
enabled, its segment files open past the point the caller believes the subsystem is gone. `shutdownTimeout` bounds the drain.

After draining the engine, the shutdown hook closes its storage, stopping SQL retention. Failed engine or SQL schema startup also closes the created
storage, so a failed build cannot leave a retention loop running.

Those hooks go into the process-wide registry by default, which runs once for the whole program. Pass a
[`runtime.HookGroup`](https://github.com/altessa-s/go-atlas-core/tree/develop/runtime) via `UseShutdownHooks` when the subsystem must be stoppable on
its own; the auditor is placed in the same scope, so stopping it stops the whole pipeline rather than half of it.

An injected dispatcher belongs to the caller and is never shut down by the builder.

SQL configuration maps purgeInterval and purgeBatchSize to the store options (defaults "1m" and 1000). A zero interval disables background purging.
Build registers shutdown through UseShutdownHooks(group) or the process-wide runtime registry and closes storage after draining dispatch. BuildStorage
callers own the returned storage and must close it before the database.
