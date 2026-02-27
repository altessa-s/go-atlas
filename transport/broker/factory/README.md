# factory

```go
import "github.com/altessa-s/go-atlas/transport/broker/factory"
```

Package `factory` provides configuration-based creation of message brokers, NATS providers, outbox, recovery, and in-progress managers
from `config` structures. When a scheduler is provided, background tasks are registered automatically.

## Factory methods

| Method                                       | Description                                                          |
|----------------------------------------------|----------------------------------------------------------------------|
| `New`                                        | Create a new `Factory` with the given options                        |
| `CreateBrokerFromConfig`                     | Create a `Broker` from configuration and a `Provider`                |
| `CreateNatsProviderWithRecoveryFromConfig`   | Create a NATS provider with optional recovery manager                |
| `CreateOutboxWithMongoFromConfig`            | Create a MongoDB-backed outbox from a `*mongo.Database`              |
| `CreateOutboxWithMongoCollectionFromConfig`  | Create a MongoDB-backed outbox from an existing `*mongo.Collection`  |
| `CreateInProgressManagerFromConfig`          | Create an InProgress heartbeat manager from configuration            |
| `CreateRecoveryManagerFromConfig`            | Create a NATS JetStream recovery manager from configuration          |

## Options

| Option                | Description                                                                   |
|-----------------------|-------------------------------------------------------------------------------|
| `WithLogger`          | Set the `*slog.Logger` for the factory (default: discard)                     |
| `WithScheduler`       | Set the task scheduler for automatic background task registration             |
| `WithPublishConverter`| Set the `PublishConverter` function applied to created brokers                 |

## Types

| Type                         | Description                                                             |
|------------------------------|-------------------------------------------------------------------------|
| `Factory`                    | Creates and configures broker components from config structures         |
| `NatsProviderWithRecovery`   | Bundles a NATS provider with its optional recovery manager              |
