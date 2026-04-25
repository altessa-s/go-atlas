# factory

```go
import "github.com/altessa-s/go-atlas/service/scheduler/factory"
```

Package `factory` provides a fluent builder for creating a `scheduler.Scheduler` from configuration.
`SchedulerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
sched, err := factory.New(cfg.Scheduler).
    UseLogger(logger).
    UseMongoDb(db).
    UseLeaderElector(elector).
    Build()
```

## Supported Storage Backends

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process (default) | — |
| `mongodb` | MongoDB | `UseMongoDb` |
| `redis` | Redis | `UseRedisClient` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `SchedulerBuilder` for the given scheduler config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseLeaderElector` | Sets the leader elector for distributed scheduling |
| `UseMongoDb` | Sets the MongoDB database for MongoDB storage backends |
| `UseRedisClient` | Sets the Redis client for Redis storage backends |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the scheduler with storage created from config |
