# brokerconfig

```go
import brokerconfig "github.com/altessa-s/go-atlas/config/broker"
```

Package `brokerconfig` defines the message broker schema with its outbox and in-progress tracking blocks. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                | Description                                                                   |
|---------------------|-------------------------------------------------------------------------------|
| `Provider`    | Defines the type of message broker to use.                                    |
| `Config`            | Defines the configuration for message broker operations.                      |
| `InProgressMetrics` | Defines the metrics configuration for InProgress heartbeat manager.           |
| `InProgress`        | Defines the configuration for InProgress heartbeat manager.                   |
| `Outbox`            | Defines the configuration for reliable event delivery via the outbox pattern. |

See the [config index](../README.md) for the other schema packages.
