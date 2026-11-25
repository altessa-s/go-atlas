# schedulerconfig

```go
import schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
```

Package `schedulerconfig` defines the distributed scheduler schema. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

## Schedule validation

| Symbol         | Description                                                                                                        |
|----------------|--------------------------------------------------------------------------------------------------------------------|
| `ScheduleRule` | ozzo rule: the value (string-kinded or a pointer to one) parses with go-horai `schedule.Parse`; nil and empty skip |

```go
validation.Field(&c.Schedule, validation.Required, schedulerconfig.ScheduleRule)
```

## Key types

| Type                      | Description                                                                             |
|---------------------------|-----------------------------------------------------------------------------------------|
| `ConcurrencyStrategy`     | Defines the concurrency strategy.                                                       |
| `MemoryAwareConcurrency`  | Configures the memory-aware concurrency strategy.                                       |
| `AdaptiveConcurrency`     | Configures the adaptive concurrency strategy.                                           |
| `Concurrency`             | Provides the strategy used by the [go-horai factory](../../factory/ext/horai).                       |
| `TaskConcurrency`         | Configures the concurrency behavior of the scheduler.                                   |
| `StorageType`             | Defines the storage backend type for the scheduler.                                     |
| `StorageMongoConfig`      | Contains MongoDB-specific settings for the scheduler storage.                           |
| `StorageRedisConfig`      | Contains Redis-specific settings for the scheduler storage.                             |
| `StorageMemoryConfig`     | Contains in-memory storage settings for the scheduler.                                  |
| `StorageSQLConfig`        | Contains SQL-specific settings for the scheduler storage (go-horai/storage/sqldb).      |
| `StorageConfig`           | Configures the scheduler storage backend.                                               |
| `HistoryStorageType`      | Defines the backend of a history storage kept apart from the task state.                |
| `HistoryClickHouseConfig` | Contains ClickHouse settings for the history storage (go-horai/storage/clickhouse).     |
| `HistoryStorageConfig`    | Moves execution history out of the task storage (`historyStorage`; nil keeps it there). |
| `Config`                  | Configures the task scheduler service.                                                  |

The `paging` section of the paginated task and history lists is the shared [`config/paging`](../paging) schema (nil = no codec).

See the [config index](../README.md) for the other schema packages.
