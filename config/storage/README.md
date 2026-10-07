# storageconfig

```go
import storageconfig "github.com/altessa-s/go-atlas/config/storage"
```

Package `storageconfig` defines shared storage-backend schemas: cache storage selection (memory, NATS, Redis) and NATS JetStream KeyValue settings.
Schemas are populated by [`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages
never import them.

## Key types

| Type                  | Description                                                         |
|-----------------------|---------------------------------------------------------------------|
| `KVStorageType`       | Selects where a NATS JetStream KeyValue bucket keeps its data.      |
| `NATSConfig`   | Defines common NATS-specific configuration for storage backends.    |
| `RedisConfig`  | Defines common Redis-specific configuration for storage backends.   |
| `MemoryConfig` | Defines common in-memory storage configuration.                     |
| `CacheStorageType`    | Defines storage backend types for caching.                          |
| `CacheStorageConfig`  | Defines storage backend configuration with type selector.           |
| `Enableable`          | Is an interface for configurations that can be enabled or disabled. |
| `IgnoreConfig`        | Defines common configuration for method/pattern-based exclusions.   |

See the [config index](../README.md) for the other schema packages.
