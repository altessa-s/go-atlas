# factory

```go
import "github.com/altessa-s/go-atlas/factory/coord/dlock"
```

Package `factory` provides a fluent builder for creating a distributed lock from configuration. `DLockBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
lock, err := factory.New(cfg.DistributionLock).
    UseLogger(logger).
    UseProvider(lockconfig.DistributionLockProviderNATS, natsfactory.Builder(natsConn)).
    Build(ctx)
```

The builder builds only the `sqldb` provider. `nats` and `mongo` use the function registered with `UseProvider`; without one `Build` fails
naming the package to add.

## Supported Providers

| Provider | Requires                                                                                                                                                                                          |
|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `nats`   | `UseProvider("nats", natsfactory.Builder(conn))` from [`factory/coord/dlock/providers/nats`](providers/nats)                                                                                 |
| `mongo`  | `UseProvider("mongo", mongofactory.Builder(db))` from [`factory/coord/dlock/providers/mongo`](providers/mongo) (`mongo.ensureIndexes: true` reconciles the cleanup TTL index during `Build`) |
| `sqldb`  | `UseSQLDB`                                                                                                                                                                                        |

## Methods

### Constructor

| Method     | Description                                                     |
|------------|-----------------------------------------------------------------|
| `New(cfg)` | Creates a `DLockBuilder` for the given distribution lock config |

### Dependencies

| Method                 | Description                                                                                                            |
|------------------------|------------------------------------------------------------------------------------------------------------------------|
| `UseLogger`            | Sets the logger for the builder, the `DLock` and the `mongo`/`sqldb` providers (the `nats` provider keeps its default) |
| `UseDefaultLogger`     | Sets the logger to `slog.Default()`                                                                                    |
| `UseProvider`          | Registers the `ProviderFunc` of a provider type the builder does not build (`nats`, `mongo`)                           |
| `UseSQLDB`             | Sets the `*sql.DB` for the `sqldb` provider                                                                            |
| `UseHealthCoordinator` | Registers the lock with a `health.Coordinator`                                                                         |
| `UseHealthServiceName` | Overrides the health service name (default `dlock`)                                                                    |

### Terminal

| Method       | Description                                |
|--------------|--------------------------------------------|
| `Build(ctx)` | Assembles and returns the distributed lock |
