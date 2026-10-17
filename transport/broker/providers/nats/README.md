# natsprovider

```go
import natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
```

Package `natsprovider` provides a NATS JetStream implementation of `broker.Provider`. Manages persistent streams, consumers, and reliable message
delivery through the JetStream API. The `NATS` type is safe for concurrent use.

## Key type

| Type   | Description                                                                                     |
|--------|-------------------------------------------------------------------------------------------------|
| `NATS` | Implements `broker.Provider` using NATS JetStream; manages subscribers and subject allowlists   |

## Constructors

| Function         | Description                                                                       |
|------------------|-----------------------------------------------------------------------------------|
| `New`            | Create a new NATS JetStream provider from a `*nats.Conn`                          |
| `NewWithContext` | Create a new provider with an explicit context for startup checks                  |

## Methods

| Method           | Description                                                                       |
|------------------|-----------------------------------------------------------------------------------|
| `Publish`        | Send a single message via JetStream                                               |
| `PublishBatch`   | Send multiple messages via JetStream                                              |
| `Subscriber`     | Create a new `broker.Subscriber` using a `SubscriberFactory`                      |
| `UnsubscribeAll` | Unsubscribe all active subscribers and clear the internal list                    |
| `Subscribers`    | Return a snapshot iterator over subscribers created through this provider since the last `UnsubscribeAll`, including inactive subscribers                        |
| `NATSConn`       | Return the underlying `*nats.Conn` for advanced use cases                         |
| `JetStream`      | Return the `jetstream.JetStream` context for advanced use cases                   |
| `CreateStream`   | Create or update a single JetStream stream                                        |
| `CreateStreams`  | Create or update several streams; returns a map of stream name to stream          |
| `DeleteStream`   | Delete a JetStream stream by name                                                 |
| `CreateConsumer` | Create or update a JetStream consumer on a stream                                 |
| `DeleteConsumer` | Delete a JetStream consumer from a stream                                         |

## Subscriber factories

| Function                          | Description                                                                 |
|-----------------------------------|-----------------------------------------------------------------------------|
| `SubscriberWithConsumer`          | `broker.SubscriberFactory` that creates subscribers with a custom consumer config |
| `SubscriberWithEphemeralConsumer` | `broker.SubscriberFactory` that creates subscribers with ephemeral consumers |
| `SubscriberWithConsumerName`      | `broker.SubscriberFactory` that uses an existing named consumer             |

Each factory panics when the provider passed to it is not a `*NATS`.

## Helpers

| Function         | Description                                                                       |
|------------------|-----------------------------------------------------------------------------------|
| `IsValidSubject` | Reports whether a subject holds only ASCII letters, digits, and hyphens (no dots or wildcards) |

## Options

| Option                | Description                                                                   |
|-----------------------|-------------------------------------------------------------------------------|
| `WithAllowedSubjects` | Restrict publish and subscribe to listed subject patterns (NATS wildcards)    |
| `WithCollector`       | Set the `metrics.Collector` for the `broker` subscriber metrics (default: no-op) |

## Errors

| Error                  | Description                                                                  |
|------------------------|------------------------------------------------------------------------------|
| `ErrSubjectNotAllowed` | Returned when a subject does not match any pattern in the allowlist          |

## Subpackages

| Package                | Description                                                                  |
|------------------------|------------------------------------------------------------------------------|
| [recovery](./recovery) | Automatic recovery for JetStream streams and consumers                       |
