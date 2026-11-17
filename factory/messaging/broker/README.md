# factory

```go
import "github.com/altessa-s/go-atlas/factory/messaging/broker"
```

Package `factory` provides a fluent builder for creating a `broker.Broker` and related components from configuration. `BrokerBuilder.Build` validates
the configuration and provider. The `Create*` helpers return their own errors immediately.

## Quick Start

```go
b := factory.New(cfg.Broker).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseProvider(brokerconfig.ProviderNATS, natsfactory.Builder(natsConn))

provider, err := b.CreateProvider()
broker, err := b.Build(provider)
```

The builder builds no broker provider itself. `CreateProvider` uses the function registered with `UseProvider` for `provider`; a `nats`
provider without one fails naming the package to add, any other type fails with an unsupported provider type error. The NATS provider and its
stream recovery manager come from [`factory/messaging/broker/providers/nats`](providers/nats).

## Methods

### Constructor

| Method     | Description                                           |
|------------|-------------------------------------------------------|
| `New(cfg)` | Creates a `BrokerBuilder` for the given broker config |

### Dependencies

| Method                | Description                                                                                       |
|-----------------------|---------------------------------------------------------------------------------------------------|
| `UseLogger`           | Sets the logger for the builder and all created components                                        |
| `UseDefaultLogger`    | Sets the logger to `slog.Default()`                                                               |
| `UseScheduler`        | Sets the task scheduler for background processes                                                  |
| `UseMeter`            | Sets the `metrics.Meter` of the broker, the provider and the in-progress manager (see Metrics)    |
| `UsePublishConverter` | Sets the converter applied to publish operations                                                  |
| `UseProvider`         | Registers the `ProviderFunc` of a provider type (`nats`) for `CreateProvider`                     |
| `UseOutboxStorage`    | Registers the outbox `StorageFunc` of a storage type (`mongo`) for `CreateOutbox`                  |
| `UseSQLDB`            | Injects the `*sql.DB` of the outbox `sqldb` storage                                               |
| `UseSQLListener`      | Injects the `outboxsql.Listener` the `sqldb` outbox store watches through with `outbox.watch: notify` |
| `UseShutdownHooks`    | Scope the outbox watcher is stopped by; defaults to the process-wide registry                     |

### Terminal

| Method            | Description                                               |
|-------------------|-----------------------------------------------------------|
| `Build(provider)` | Assembles and returns the broker using the given provider |

### Helpers

| Method                                               | Description                                                                                                                                                                              |
|------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `CreateInProgressManager()`                          | Creates an in-progress heartbeat manager from the builder's broker config; registers background tick task when a scheduler is set; returns `nil, nil` when `inProgress.enabled` is false |
| `CreateProvider()`                                   | Creates the broker provider of the configured type with the registered `ProviderFunc`                                                                                                    |
| `CreateOutbox(ctx, publisher)`                       | Creates a broker outbox over the storage named by `outbox.storage.type`: `sqldb` over `UseSQLDB` (the table is created only with `storage.sqldb.ensureSchema: true`), `mongo` over the function from [`factory/reliability/outbox/storages/mongo`](../../reliability/outbox/storages/mongo); starts the watcher for `outbox.watch: changeStream` or `notify` |

## Types

| Type           | Description                                                                               |
|----------------|-------------------------------------------------------------------------------------------|
| `ProviderFunc` | Creates the broker provider of one provider type from the broker config and `ProviderEnv` |
| `ProviderEnv`  | The builder's logger, scheduler and meter, handed to a `ProviderFunc`                     |

## Metrics

`UseMeter` feeds three components: the broker built by `Build` (publish metrics), the provider built by `CreateProvider` (the NATS subscriber
metrics, through `ProviderEnv.Meter`) and the in-progress manager. Only the in-progress manager has a switch, `inProgress.metrics.enabled`;
the broker and the provider record whenever a meter is set. Without a meter all three record nothing.

## Outbox startup

`CreateOutbox` delegates to [`factory/reliability/outbox`](../../reliability/outbox) with the broker adapter's handler and retry predicate, so both
factories read the `outbox` config identically. Its `ctx` bounds the startup I/O (schema creation, task registration). It returns `nil, nil` when
`outbox.enabled` is false, before any storage is built; an enabled outbox requires a publisher. Outbox builders call `RegisterTasks` before returning.
A scheduler registration failure returns both a non-nil outbox and an error. Retain that instance to retry `RegisterTasks(ctx)`; callbacks already
installed remain inactive until all configured tasks are registered. Do not start serving traffic while registration is incomplete.
