# retryqueueconfig

```go
import retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
```

Package `retryqueueconfig` defines the retry queue schema: the backend that persists retry items. Schemas are populated by [`go-
nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by
[`factory/reliability/retryqueue`](../../factory/reliability/retryqueue), which maps them to generated options; runtime packages never import them.

## Key types

| Type                  | Description                                                                                         |
|-------------------------|-------------------------------------------------------------------------------------------------------|
| `StorageType`         | Selects the backend: `memory`, `mongo` or `sqldb`.                                                  |
| `MemoryStorageConfig` | Configures the in-process store (reliability/retryqueue/storages/memory).                           |
| `MongoStorageConfig`  | Collection, flag field names, resolved retention and index creation of the MongoDB store.           |
| `SQLStorageConfig`    | Dialect, table, schema creation, resolved retention and background purge interval of the SQL store. |
| `StorageConfig`       | Selects and configures the backend; only the block named by `type` is used.                         |
| `Config`              | The retry queue configuration: its `storage` block.                                                 |

The template is [`templates/retryqueue.yaml`](../templates/retryqueue.yaml). See the [config index](../README.md) for the other schema packages.

The sqldb block exposes purgeInterval (default "1m", "0s" disables the background loop) and purgeBatchSize (default 1000). Manual PurgeExpired remains
available with a zero interval. Purging runs only when retention is enabled.
