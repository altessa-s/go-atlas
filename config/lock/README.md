# lockconfig

```go
import lockconfig "github.com/altessa-s/go-atlas/config/lock"
```

Package `lockconfig` defines distributed lock and leader election schemas. Schemas are populated by [`go-nomos/loader`](https://github.com/altessa-s/go-
nomos/tree/develop/loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                       | Description                                                         |
|------------------------------|-----------------------------------------------------------------------|
| `DistributionLockProvider` | Defines the distributed locking provider type.                      |
| `DistributionLockMongo`  | Defines the MongoDB-specific configuration for distributed locking. |
| `DistributionLockNATS`     | Defines the NATS-specific configuration for distributed locking.    |
| `DistributionLockSQL`      | Defines the SQL configuration: dialect, table, lease and cleanup.   |
| `DistributionLock`         | Defines the configuration for distributed locking.                  |
| `LeaderElectorProvider`    | Defines the type of leader election provider.                       |
| `LeaderElector`            | Defines the configuration for distributed leader election.          |

See the [config index](../README.md) for the other schema packages.

The sqldb block exposes `purgeInterval` (default `"1m"`, `"0s"` disables the background loop) and `purgeBatchSize` (default 1000). Manual `PurgeExpired`
remains available with a zero interval. Purging runs only when `expiredCleanup` is enabled.
