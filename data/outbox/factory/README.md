# factory

```go
import "github.com/altessa-s/go-atlas/data/outbox/factory"
```

Package `factory` provides a fluent builder for creating an `outbox.Outbox` from configuration.
`OutboxBuilder` uses deferred error accumulation — errors from any step are collected and returned at build time.

## Quick Start

```go
outbox, err := factory.New(cfg.Outbox).
    UseLogger(logger).
    UseScheduler(scheduler).
    BuildWithMongoDB(db, handler)
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates an `OutboxBuilder` for the given outbox config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseScheduler` | Sets the task scheduler for background dispatch, unlock, and cleanup processes |

### Terminal

| Method | Description |
|--------|-------------|
| `BuildWithMongoDB(db, handler)` | Creates a MongoDB-backed outbox using a `*mongo.Database` |
| `BuildWithMongoCollection(col, handler)` | Creates a MongoDB-backed outbox using an existing `*mongo.Collection` |
