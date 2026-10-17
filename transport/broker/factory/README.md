# factory

```go
import "github.com/altessa-s/go-atlas/transport/broker/factory"
```

Package `factory` provides a fluent builder for creating a `broker.Broker` and related components from configuration. `BrokerBuilder.Build` validates
the configuration and provider. The `Create*` helpers return their own errors immediately.

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
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseScheduler` | Sets the task scheduler for background processes |
| `UsePublishConverter` | Sets the converter applied to publish operations |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(provider)` | Assembles and returns the broker using the given provider |

### Helpers

| Method | Description |
|--------|-------------|
| `CreateInProgressManager()` | Creates an in-progress heartbeat manager from the builder's broker config; registers background tick task when a scheduler is set; returns `nil, nil` when `inProgress.enabled` is false |
| `CreateOutboxWithMongoDB(db, publisher)` | Creates a MongoDB-backed broker outbox using a `*mongo.Database` |
| `CreateOutboxWithMongoCollection(col, publisher)` | Creates a MongoDB-backed broker outbox using an existing `*mongo.Collection` |
| `CreateOutboxWithSQLDB(db, dialect, publisher)` | Creates a SQL-backed broker outbox (PostgreSQL, MySQL or MariaDB) using a `*sql.DB`; creates the events table only with `outbox.ensureSchema: true` (otherwise it must exist) |
| `CreateNATSProviderWithRecovery(conn)` | Creates a NATS provider bundled with an optional recovery manager; returns `NATSProviderWithRecovery`. The manager is not started: call `Recovery.Start()` after `RegisterStream` |
| `CreateRecoveryManager(provider)` | Creates a NATS JetStream recovery manager from the builder's broker config; returns `nil, nil` if recovery is disabled |

## Types

| Type | Description |
|------|-------------|
| `NATSProviderWithRecovery` | Bundles a `*natsprovider.NATS` provider with its optional `*recovery.Manager`; call `Close()` to stop the recovery manager |

## Outbox startup

The `CreateOutboxWith*` helpers return `nil, nil` when `outbox.enabled` is false, but only after dependency validation and storage construction (MongoDB
index creation included) succeed — check `outbox.enabled` first to skip them. Outbox builders call `RegisterTasks` before returning. A scheduler
registration failure returns both a non-nil outbox and an error. Retain that instance to retry `RegisterTasks(ctx)`; callbacks already installed remain
inactive until all configured tasks are registered. Do not start serving traffic while registration is incomplete.
