# factory

```go
import "github.com/altessa-s/go-atlas/factory/coord/leadelect"
```

Package `factory` provides a fluent builder for creating a leader elector from configuration. `LeaderBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
leader, err := factory.New(cfg.LeaderElector).
    UseLogger(logger).
    UseProvider(lockconfig.LeaderElectorProviderNATS, natsfactory.Builder(natsConn)).
    WithNodeId(nodeID). // node ID required by Build
    Build(ctx)
```

The builder builds no provider itself. `nats` (also an empty `provider`) uses the function registered with `UseProvider` from
[`factory/coord/leadelect/providers/nats`](providers/nats); without one `Build` fails naming the package to add.

## Methods

### Constructor

| Method     | Description                                                                                   |
|------------|-----------------------------------------------------------------------------------------------|
| `New(cfg)` | Creates a `LeaderBuilder` for the given leader elector config. Defaults key to `appinfo.Name` |

### Dependencies

| Method             | Description                                                |
|--------------------|------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                        |
| `UseProvider`      | Registers the `ProviderFunc` of a provider type (`nats`)   |
| `UseMeter`         | Sets the metrics collector                                 |

### Configuration

| Method          | Description                                                                                 |
|-----------------|---------------------------------------------------------------------------------------------|
| `WithKey(v)`    | Sets the leader election key (default: `appinfo.Name`)                                      |
| `WithNodeId(v)` | Sets this replica's unique node identifier (required by `Build`) |

### Terminal

| Method       | Description                              |
|--------------|------------------------------------------|
| `Build(ctx)` | Assembles and returns the leader elector |
