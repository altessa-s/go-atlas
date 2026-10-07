# schedulerconfig

```go
import schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
```

Package `schedulerconfig` defines the distributed scheduler schema. Schemas are populated by [`config/loader`](../loader) and consumed by the component
factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                           | Description                                                                                  |
|--------------------------------|----------------------------------------------------------------------------------------------|
| `ConcurrencyStrategy`          | Defines the concurrency strategy.                                                            |
| `MemoryAwareConcurrency`       | Configures the memory-aware concurrency strategy.                                            |
| `AdaptiveConcurrency`          | Configures the adaptive concurrency strategy.                                                |
| `Concurrency`                  | Is the base concurrency configuration reusable across components.                            |
| `TaskConcurrency`         | Configures the concurrency behavior of the scheduler.                                        |
| `StorageType`         | Defines the storage backend type for the scheduler.                                          |
| `StorageMongoConfig`  | Contains MongoDB-specific settings for the scheduler storage.                                |
| `StorageRedisConfig`  | Contains Redis-specific settings for the scheduler storage.                                  |
| `StorageMemoryConfig` | Contains in-memory storage settings for the scheduler.                                       |
| `StorageSQLConfig`    | Contains SQL-specific settings for the scheduler storage (service/scheduler/storages/sqldb). |
| `StorageConfig`       | Configures the scheduler storage backend.                                                    |
| `HistoryStorageType`      | Defines the backend of a history storage kept apart from the task state.                 |
| `HistoryClickHouseConfig` | Contains ClickHouse settings for the history storage (service/scheduler/storages/clickhouse). |
| `HistoryStorageConfig`    | Moves execution history out of the task storage (`historyStorage`; nil keeps it there).   |
| `Config`                    | Configures the task scheduler service.                                                       |

See the [config index](../README.md) for the other schema packages.
