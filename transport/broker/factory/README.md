# factory

```go
import "github.com/altessa-s/go-atlas/transport/broker/factory"
```

Package `factory` provides a fluent builder for creating a `broker.Broker` and related components from configuration.
`BrokerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
b := factory.New(cfg.Broker).
    UseLogger(logger).
    UseScheduler(scheduler)

broker, err := b.Build(provider)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `BrokerBuilder` for the given broker config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseScheduler` | Sets the task scheduler for background processes |
| `UsePublishConverter` | Sets the converter applied to publish operations |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(provider)` | Assembles and returns the broker using the given provider |

### Helpers

| Method | Description |
|--------|-------------|
| `CreateInProgressManager()` | Creates an in-progress heartbeat manager from the builder's broker config; registers background tick task when a scheduler is set |
| `CreateOutboxWithMongoDB(db, publisher)` | Creates a MongoDB-backed broker outbox using a `*mongo.Database` |
| `CreateOutboxWithMongoCollection(col, publisher)` | Creates a MongoDB-backed broker outbox using an existing `*mongo.Collection` |
| `CreateNatsProviderWithRecovery(conn)` | Creates a NATS provider bundled with an optional recovery manager; returns `NatsProviderWithRecovery` |
| `CreateRecoveryManager(provider)` | Creates a NATS JetStream recovery manager from the builder's broker config; returns `nil, nil` if recovery is disabled |

## Types

| Type | Description |
|------|-------------|
| `NatsProviderWithRecovery` | Bundles a `*natsprovider.Nats` provider with its optional `*recovery.Manager`; call `Close()` to stop the recovery manager |
