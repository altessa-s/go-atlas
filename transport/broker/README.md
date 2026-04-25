# broker

```go
import "github.com/altessa-s/go-atlas/transport/broker"
```

Package `broker` provides a high-level message broker abstraction for reliable asynchronous communication. Supports multiple messaging
systems and implements the Transactional Outbox pattern for exactly-once delivery semantics in distributed systems.

## Key types

| Type                | Description                                                                                    |
|---------------------|------------------------------------------------------------------------------------------------|
| `Broker`            | Main entry point combining publishing and subscriber creation; safe for concurrent use         |
| `Provider`          | Interface for transport-specific implementations (e.g. NATS JetStream)                        |
| `Subscriber`        | Interface for message subscription lifecycle: `Subscribe`, `Unsubscribe`, `Closed`             |
| `Publisher`         | Interface for sending messages: `Publish`, `PublishBatch`, `PublishAny`                        |
| `Outboxer`          | Interface for transactional outbox pattern ensuring reliable delivery                          |
| `PublishConverter`   | Function type that converts arbitrary types to `msg.Message` for use with `PublishAny`         |
| `SubscriberFactory` | Function type that creates a new `Subscriber` from a provider                                  |
| `SubscriberHandler` | Interface for message processing: `Handle` and `Topic`                                         |

## Methods

| Method        | Description                                                                                    |
|---------------|------------------------------------------------------------------------------------------------|
| `New`         | Create a new `Broker` with a `Provider` and optional configuration                            |
| `Publish`     | Send a single message via the configured `Outboxer`                                            |
| `PublishBatch`| Send multiple messages in a single batch operation                                             |
| `PublishAny`  | Convert arbitrary types to messages using `PublishConverter` and publish them                   |
| `Subscriber`  | Create a new `Subscriber` using a `SubscriberFactory`                                          |

## Options

| Option                | Description                                                                   |
|-----------------------|-------------------------------------------------------------------------------|
| `WithLogger`          | Set the `*slog.Logger` for the broker (default: discard)                      |
| `WithOutbox`          | Set a custom `Outboxer` for reliable delivery (default: direct publish)       |
| `WithPublishConverter`| Set the `PublishConverter` function for `PublishAny`                           |

## Errors

| Error                      | Description                                                              |
|----------------------------|--------------------------------------------------------------------------|
| `ErrPublishConverterNotSet`| Returned by `PublishAny` when no `PublishConverter` has been configured   |

## Subpackages

| Package                            | Description                                                        |
|------------------------------------|--------------------------------------------------------------------|
| [factory](./factory)               | Configuration-based creation of brokers and related components     |
| [inprogress](./inprogress)         | Periodic InProgress heartbeat manager for long-running tasks       |
| [msg](./msg)                       | Message envelope with topic, payload, metadata, and acknowledgment |
| [outbox](./outbox)                 | Broker-specific adapter for the transactional outbox pattern       |
| [providers/nats](./providers/nats) | NATS JetStream implementation of `Provider`                        |
