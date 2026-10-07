# natsconfig

```go
import natsconfig "github.com/altessa-s/go-atlas/config/nats"
```

Package `natsconfig` defines the NATS connection, JetStream consumer and recovery schemas. Schemas are populated by [`config/loader`](../loader) and
consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type            | Description                                                                  |
|-----------------|------------------------------------------------------------------------------|
| `Recovery`  | Defines configuration for automatic NATS JetStream stream/consumer recovery. |
| `Config`          | Represents the configuration for NATS messaging system connections.          |
| `Consumers` | Represents a collection of NATS JetStream consumer configurations.           |
| `DeliverPolicy` | Defines the point in the stream to start delivering messages.                |
| `AckPolicy`     | Defines how messages should be acknowledged.                                 |
| `ReplayPolicy`  | Defines how messages are replayed from the stream.                           |
| `Consumer`  | Represents the configuration for a NATS JetStream consumer.                  |

See the [config index](../README.md) for the other schema packages.
