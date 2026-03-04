# factory

```go
import "github.com/altessa-s/go-atlas/data/leadelect/factory"
```

Package `factory` provides a fluent builder for creating a leader elector from configuration.
`LeaderBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
leader, err := factory.New(cfg.LeaderElector).
    UseLogger(logger).
    UseNatsConn(natsConn).
    Build(ctx)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `LeaderBuilder` for the given leader elector config. Defaults key to `appinfo.Name` |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseNatsConn` | Sets the NATS connection used for leader election |

### Configuration

| Method | Description |
|--------|-------------|
| `WithKey(v)` | Sets the leader election key (default: `appinfo.Name`) |
| `WithNodeId(v)` | Sets the node identifier for leader election |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the leader elector |
