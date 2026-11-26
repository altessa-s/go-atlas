# brokerconfig

```go
import brokerconfig "github.com/altessa-s/go-atlas/config/broker"
```

Package `brokerconfig` defines the message broker schema with its outbox and in-progress tracking blocks. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

## Key types

| Type                  | Description                                                                                          |
|-----------------------|------------------------------------------------------------------------------------------------------|
| `Provider`            | Defines the type of message broker to use.                                                           |
| `Config`              | Defines the configuration for message broker operations.                                             |
| `InProgressMetrics`   | Defines the metrics configuration for InProgress heartbeat manager.                                  |
| `InProgress`          | Defines the configuration for InProgress heartbeat manager.                                          |

The `outbox` block uses the [outbox schema](../outbox). `Config.Normalize` prepares its storage settings before validation.

See the [config index](../README.md) for the other schema packages.
