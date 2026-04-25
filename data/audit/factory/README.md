# factory

```go
import "github.com/altessa-s/go-atlas/data/audit/factory"
```

Package `factory` provides a fluent builder for creating an audit auditor from configuration.
`AuditorBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
auditor, err := factory.New(cfg.Audit).
    UseLogger(logger).
    UseDispatcher(eng).
    Build()
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates an `AuditorBuilder` for the given audit config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseDispatcher` | Sets the `audit.Dispatcher` (must already be started) |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles, starts, and returns the audit auditor |
