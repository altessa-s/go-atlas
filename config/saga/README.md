# sagaconfig

```go
import sagaconfig "github.com/altessa-s/go-atlas/config/saga"
```

Package `sagaconfig` defines the saga orchestration schema. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

## Key types

| Type                      | Description                                                                                             |
|---------------------------|---------------------------------------------------------------------------------------------------------|
| `StorageType`         | Selects the backend that persists saga instances.                                                       |
| `MemoryStorageConfig` | Configures the in-process saga store (reliability/saga/storages/memory).                                       |
| `NATSStorageConfig`   | Configures the durable saga store backed by a NATS JetStream KeyValue bucket (reliability/saga/storages/nats). |
| `MongoStorageConfig`  | Configures the durable saga store backed by a MongoDB collection (reliability/saga/storages/mongo).            |
| `RedisStorageConfig`  | Configures the durable saga store backed by Redis (reliability/saga/storages/redis).                           |
| `SQLStorageConfig`    | Configures the durable saga store backed by a SQL table (reliability/saga/storages/sqldb).                     |
| `StorageConfig`       | Selects and configures the saga state-store backend.                                                    |
| `Config`                    | Configures the saga orchestrator and its state store.                                                   |

See the [config index](../README.md) for the other schema packages.
