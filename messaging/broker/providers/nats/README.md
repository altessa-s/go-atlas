# natsprovider

```go
import natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
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
| `DefaultSubscriber` | Create a tracked ephemeral subscriber |
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
| `SubscriberWithConsumer`          | `SubscriberFactory` that creates subscribers with a custom consumer config |
| `SubscriberWithEphemeralConsumer` | `SubscriberFactory` that creates subscribers with ephemeral consumers |
| `SubscriberWithConsumerName`      | `SubscriberFactory` that uses an existing named consumer             |

Each factory accepts `*NATS` directly (`func(*NATS) broker.Subscriber`), enforced at compile time.

## Helpers

| Function         | Description                                                                       |
|------------------|-----------------------------------------------------------------------------------|
| `IsValidSubject` | Reports whether a subject holds only ASCII letters, digits, and hyphens (no dots or wildcards) |

## Options

| Option                | Description                                                                   |
|-----------------------|-------------------------------------------------------------------------------|
| `WithAllowedSubjects` | Restrict publish and subscribe to listed subject patterns (NATS wildcards)    |
| `WithMeter`           | Set the `metrics.Meter` for the `broker` subscriber metrics (default: no-op)     |
| `WithDrainTimeout`    | Grace period `Unsubscribe` gives handlers of buffered messages before canceling their context (default: `DefaultDrainTimeout`, 30s) |

## Errors

| Error                  | Description                                                                  |
|------------------------|------------------------------------------------------------------------------|
| `ErrSubscribeSuperseded` | A later `Subscribe` or `Unsubscribe` superseded `Subscribe` while draining; no new consumer was installed (match with `errors.Is`) |
| `ErrSubjectNotAllowed` | Returned when a subject does not match any pattern in the allowlist          |

`Publish` and `PublishBatch` wrap a failure on a closed connection with `broker.ErrConnectionClosed`; the synchronous path also keeps
`nats.ErrConnectionClosed` in the chain.

## Subpackages

| Package                | Description                                                                  |
|------------------------|------------------------------------------------------------------------------|
| [recovery](./recovery) | Automatic recovery for JetStream streams and consumers                       |
