# brokerconfig

```go
import brokerconfig "github.com/altessa-s/go-atlas/config/broker"
```

Package `brokerconfig` defines the message broker schema with its outbox and in-progress tracking blocks. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

## Key types

| Type                | Description                                                                   |
|---------------------|-------------------------------------------------------------------------------|
| `Provider`    | Defines the type of message broker to use.                                    |
| `Config`            | Defines the configuration for message broker operations.                      |
| `InProgressMetrics` | Defines the metrics configuration for InProgress heartbeat manager.           |
| `InProgress`        | Defines the configuration for InProgress heartbeat manager.                   |
| `Outbox`            | Defines the configuration for reliable event delivery via the outbox pattern. |
| `OutboxStorageConfig` | Selects the outbox storage: `type: mongo` (`mongo: {collection}`) or `type: sqldb` (`sqldb: {dialect, table, ensureSchema}`). |
| `OutboxWatch`       | `polling` (default), `changeStream` (mongo only) or `notify` (sqldb with the postgres dialect only).                   |

`Outbox.Normalize` defaults a missing `storage` block to MongoDB and allocates the backend block its type names; call it before `Validate`. A watch
mode the storage cannot serve fails validation with `ErrUnsupportedOutboxWatch`, wrapped with go-atlas-core `ErrInvalidArgument`.

See the [config index](../README.md) for the other schema packages.
