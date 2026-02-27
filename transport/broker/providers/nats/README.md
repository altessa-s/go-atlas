# natsprovider

```go
import natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
```

Package `natsprovider` provides a NATS JetStream implementation of `broker.Provider`. Manages persistent streams, consumers, and
reliable message delivery through the JetStream API. The `Nats` type is safe for concurrent use.

## Key type

| Type   | Description                                                                                     |
|--------|-------------------------------------------------------------------------------------------------|
| `Nats` | Implements `broker.Provider` using NATS JetStream; manages subscribers and subject allowlists   |

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
| `Subscribers`    | Return an iterator over active subscribers (snapshot-based)                        |
| `NatsConn`       | Return the underlying `*nats.Conn` for advanced use cases                         |
| `JetStream`      | Return the `jetstream.JetStream` context for advanced use cases                   |

## Options

| Option                | Description                                                                   |
|-----------------------|-------------------------------------------------------------------------------|
| `WithAllowedSubjects` | Restrict publish and subscribe to listed subject patterns (NATS wildcards)    |

## Errors

| Error                  | Description                                                                  |
|------------------------|------------------------------------------------------------------------------|
| `ErrSubjectNotAllowed` | Returned when a subject does not match any pattern in the allowlist          |

## Subpackages

| Package                | Description                                                                  |
|------------------------|------------------------------------------------------------------------------|
| [recovery](./recovery) | Automatic recovery for JetStream streams and consumers                       |
