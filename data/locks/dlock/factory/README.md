# factory

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/factory"
```

Package `factory` provides a fluent builder for creating a distributed lock from configuration. `DLockBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
lock, err := factory.New(cfg.DistributionLock).
    UseLogger(logger).
    UseNATSConn(natsConn).
    Build(ctx)
```

## Supported Providers

| Provider | Requires |
|----------|----------|
| `nats` | `UseNATSConn` |
| `mongo` | `UseMongoDB` |
| `sqldb` | `UseSQLDB` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `DLockBuilder` for the given distribution lock config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder, the `DLock` and the `mongo`/`sqldb` providers (the `nats` provider keeps its default) |
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseNATSConn` | Sets the NATS connection used for distributed locking |
| `UseMongoDB` | Sets the MongoDB database for the `mongo` provider |
| `UseSQLDB` | Sets the `*sql.DB` for the `sqldb` provider |
| `UseHealthCoordinator` | Registers the lock with a `health.Coordinator` |
| `UseHealthServiceName` | Overrides the health service name (default `dlock`) |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the distributed lock |
