# factory

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/factory"
```

Package `factory` provides a fluent builder for creating a distributed lock from configuration.
`DLockBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
lock, err := factory.New(cfg.DistributionLock).
    UseLogger(logger).
    UseNatsConn(natsConn).
    Build(ctx)
```

## Supported Providers

| Provider | Requires |
|----------|----------|
| `nats` | `UseNatsConn` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `DLockBuilder` for the given distribution lock config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseNatsConn` | Sets the NATS connection used for distributed locking |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the distributed lock |
