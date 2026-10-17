# factory

```go
import "github.com/altessa-s/go-atlas/data/leadelect/factory"
```

Package `factory` provides a fluent builder for creating a leader elector from configuration. `LeaderBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
leader, err := factory.New(cfg.LeaderElector).
    UseLogger(logger).
    UseNATSConn(natsConn).
    WithNodeId(nodeID). // unique per replica; Start fails on an empty node ID
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
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseNATSConn` | Sets the NATS connection used for leader election |
| `UseCollector` | Sets the metrics collector |

### Configuration

| Method | Description |
|--------|-------------|
| `WithKey(v)` | Sets the leader election key (default: `appinfo.Name`) |
| `WithNodeId(v)` | Sets this replica's unique node identifier (required: no default, `Start` fails when empty) |

### Terminal

| Method | Description |
|--------|-------------|
| `Build(ctx)` | Assembles and returns the leader elector |
