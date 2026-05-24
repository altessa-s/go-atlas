# factory

```go
import "github.com/altessa-s/go-atlas/plugins/factory"
```

Package `factory` provides a fluent builder for creating plugin managers from configuration.
`ManagerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
manager, err := factory.NewManager(cfg.Plugins).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    Build(ctx)
```

With custom health service name:

```go
manager, err := factory.NewManager(cfg.Plugins).
    UseLogger(logger).
    UseHealthCoordinator(healthCoordinator).
    UseHealthServiceName("custom-plugins").
    Build(ctx)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `NewManager(cfg)` | Creates a `ManagerBuilder` for the given plugins config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the manager |
| `UseHealthCoordinator` | Registers the manager with a health coordinator |
| `UseHealthServiceName` | Overrides the health check service name (default: "plugins") |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the plugin manager |

## Configuration

The builder consumes a [`config.Plugins`](../../config/plugins.go) struct which controls:

- Plugin directory path
- Signature verification settings
- Host version enforcement
- Quarantine policies

## Health Integration

When a health coordinator is provided, the manager automatically registers
health checks for plugin loading and verification status. The health check
monitors plugin states and reports degraded status on load failures.