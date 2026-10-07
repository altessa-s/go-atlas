# factory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/factory"
```

Package `factory` provides a fluent builder for creating a `scheduler.Scheduler` from configuration.
`SchedulerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
sched, err := factory.New(cfg.Scheduler).
    UseLogger(logger).
    UseMongoDb(db).
    UseLeaderElector(elector).
    Build()
```

The builder maps every `schedulerconfig.Config` field to a scheduler option, including `instanceID` → `scheduler.WithInstanceID` (empty keeps the
per-process random ID).

## Supported Storage Backends

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process (default) | — |
| `mongodb` | MongoDB | `UseMongoDb` |
| `redis` | Redis | `UseRedisClient` |
| `sql` | PostgreSQL / MySQL / MariaDB (`storage.sql.dialect`) | `UseSQLDB` |

`historyStorage` moves execution history to a backend of its own (`scheduler.WithHistoryStorage`); task state stays in `storage`.

| Type | Backend | Requires |
|------|---------|----------|
| `clickhouse` | ClickHouse table; a zero `ttl` takes `historyRetention` | `UseClickHouseConn` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `SchedulerBuilder` for the given scheduler config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseLeaderElector` | Sets the leader elector for distributed scheduling |
| `UseMongoDb` | Sets the MongoDB database for MongoDB storage backends (set `storage.mongodb.ensureIndexes: true` to create the indexes during `Build`; otherwise call `EnsureIndexes` on the storage first) |
| `UseRedisClient` | Sets the Redis client for Redis storage backends (set `storage.redis.ensureIndexes: true` to create the RediSearch indexes during `Build`; otherwise call `EnsureIndexes` on the storage first) |
| `UseSQLDB` | Sets the `*sql.DB` handle for SQL storage backends (set `storage.sql.ensureSchema: true` to create the schema during `Build`; otherwise create it first: `EnsureSchema` on a `sqldb.New` storage with the same handle, dialect and tables, or migrations) |
| `UseClickHouseConn` | Sets the ClickHouse connection for the ClickHouse history storage (set `historyStorage.clickhouse.ensureSchema: true` to create the table during `Build`; otherwise create it first: `EnsureSchema` on the storage, or `SchemaDDL` through migrations) |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the scheduler with storage created from config |
