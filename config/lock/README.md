# lockconfig

```go
import lockconfig "github.com/altessa-s/go-atlas/config/lock"
```

Package `lockconfig` defines distributed lock and leader election schemas. Schemas are populated by [`config/loader`](../loader) and consumed by the
component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                       | Description                                                         |
|----------------------------|---------------------------------------------------------------------|
| `DistributionLockProvider` | Defines the distributed locking provider type.                      |
| `DistributionLockMongodb`  | Defines the MongoDB-specific configuration for distributed locking. |
| `DistributionLockNats`     | Defines the NATS-specific configuration for distributed locking.    |
| `DistributionLock`         | Defines the configuration for distributed locking.                  |
| `LeaderElectorProvider`    | Defines the type of leader election provider.                       |
| `LeaderElector`            | Defines the configuration for distributed leader election.          |

See the [config index](../README.md) for the other schema packages.
