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
    UseMongoDb(db).
    Build()
```

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process slice | — |
| `mongo` | MongoDB collection | `UseMongoDb` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates an `AuditorBuilder` for the given audit config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseMongoDb` | Sets the MongoDB database for Mongo storage backends |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles, starts, and returns the audit auditor |
