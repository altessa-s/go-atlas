# broker

```go
import "github.com/altessa-s/go-atlas/messaging/broker"
```

Package `broker` provides a high-level message broker abstraction for reliable asynchronous communication. Supports multiple messaging systems and
implements the Transactional Outbox pattern for at-least-once delivery in distributed systems; consumers deduplicate repeats by the stable event ID.

## Key types

| Type                | Description                                                                                    |
|---------------------|------------------------------------------------------------------------------------------------|
| `Broker`            | Main entry point combining publishing and subscriber creation; safe for concurrent use         |
| `Brokerer`          | Interface implemented by `Broker`: `Publisher` plus `Subscriber` creation                      |
| `Provider`          | Interface for transport-specific implementations (e.g. NATS JetStream)                        |
| `Subscriber`        | Interface for message subscription lifecycle: `Subscribe`, `Unsubscribe`, `Closed`             |
| `Publisher`         | Interface for sending messages: `Publish`, `PublishBatch`, `PublishAny`                        |
| `Outboxer`          | Interface for transactional outbox pattern ensuring reliable delivery                          |
| `PublishConverter`   | Function type that converts arbitrary types to `msg.Message` for use with `PublishAny`         |
| `SubscriberFactory` | Creates a subscriber using a provider bound in the closure. Pass nil for the default                                  |
| `SubscriberHandler` | Interface for message processing: `Handle` and `Topic`                                         |

## Methods

| Method        | Description                                                                                    |
|---------------|------------------------------------------------------------------------------------------------|
| `New`         | Create a new `Broker` with a `Provider` and optional configuration                            |
| `Publish`     | Send a single message via the configured `Outboxer`                                            |
| `PublishBatch`| Send multiple messages in a single batch operation                                             |
| `PublishAny`  | Convert arbitrary types to messages using `PublishConverter` and publish them                   |
| `Subscriber`  | Create a new `Subscriber` using a `SubscriberFactory`                                          |

A nil factory uses `Provider.DefaultSubscriber`. For a named NATS consumer, bind the provider in the factory. Calling the provider’s `Subscriber`
method registers the subscriber for shutdown, and the broker applies its tracing wrapper.

```go
sub := b.Subscriber(func() broker.Subscriber {
    return provider.Subscriber(natsprovider.SubscriberWithConsumerName("orders"))
})
```

## Options

| Option                  | Description                                                                                          |
|-------------------------|------------------------------------------------------------------------------------------------------|
| `WithMeter`             | Set the `metrics.Meter` for the `broker` publish metrics (default: no-op)                            |
| `WithLogger`            | Set the `*slog.Logger` for the broker (default: discard)                                             |
| `WithOutbox`            | Set a custom `Outboxer` for reliable delivery (default: direct publish)                              |
| `WithPublishConverter`  | Set the `PublishConverter` function for `PublishAny`                                                 |
| `WithSubjectLabelLimit` | Max distinct `subject` label values on publish metrics; extra subjects go to `_other` (default: 256) |

## Distributed tracing

`Publish`, `PublishBatch` and `PublishAny` copy a valid W3C context from the request to lowercase `traceparent` and optional `tracestate` metadata.
This includes unsampled requests. An explicit message `traceparent` takes precedence. Publishing adds no span and requires no broker tracer.

Configure `broker.WithTracer(tracer)` to trace handlers registered through `Broker.Subscriber`. A CONSUMER span named `process <name>` lasts
for the call to `Handle`. NATS supplies the configured durable or consumer name. Other subscribers may implement `ConsumerName() string`.
`broker.WithConsumerName(name)` sets an explicit name for this broker. The fallback is `subscriber`. Use a low-cardinality name.
`broker.WithMessagingSystem("nats")` overrides the system reported by the provider. NATS reports `nats`. The span carries the destination topic, consumer group
and deduplication ID when available. A Nak, NakWithBackOff or Term during the call marks the span as an error.

A valid sampled remote context parents the consumer span. An unsampled remote context reaches the handler unchanged, so subsequent publishes and
outgoing calls propagate its sampling flag. A message with absent or invalid context starts a new root subject to the local sampler.

Subscriptions made directly through a provider bypass `Broker.Subscriber`. Pass the wrapped handler to the NATS recovery manager.

```go
handler := broker.TraceHandler(eventHandler, tracer, durable, "nats")
sub, err := provider.Recovery.Subscribe(ctx, stream, durable, handler, factory)
```

## Errors

| Error                      | Description                                                              |
|----------------------------|--------------------------------------------------------------------------|
| `ErrPublishConverterNotSet`| Returned by `PublishAny` when no `PublishConverter` has been configured   |
| `ErrConnectionClosed`      | Provider-neutral mark for a publish on a permanently closed connection   |

## Subpackages

| Package                                   | Description                                                        |
|-------------------------------------------|--------------------------------------------------------------------|
| [factory](../../factory/messaging/broker) | Configuration-based creation of brokers and related components     |
| [idempotency](./idempotency)              | Consumer deduplication middleware over at-least-once delivery      |
| [inprogress](./inprogress)                | Periodic InProgress heartbeat manager for long-running tasks       |
| [msg](./msg)                              | Message envelope with topic, payload, metadata, and acknowledgment |
| [outbox](./outbox)                        | Broker-specific adapter for the transactional outbox pattern       |
| [providers/nats](./providers/nats)        | NATS JetStream implementation of `Provider`                        |
