# factory

```go
import "github.com/altessa-s/go-atlas/auth/opa/factory"
```

Package `factory` provides a fluent builder for creating OPA managers from configuration.
`ManagerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
manager, err := factory.New(cfg.OPA).
    UseLogger(logger).
    UseScheduler(scheduler).
    UseHealthCoordinator(hc).
    Build(ctx)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ManagerBuilder` for the given OPA config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseScheduler` | Sets the task registrar for periodic policy update cycles |
| `UseHealthCoordinator` | Sets the health coordinator for manager health reporting |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the OPA manager |
