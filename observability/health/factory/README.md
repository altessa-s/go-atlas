# factory

```go
import "github.com/altessa-s/go-atlas/observability/health/factory"
```

Package `factory` provides a fluent builder for creating health coordinators from configuration.
`CoordinatorBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
coordinator, err := factory.New(cfg.Health).
    UseLogger(logger).
    Build()
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `CoordinatorBuilder` for the given health config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the health coordinator |
