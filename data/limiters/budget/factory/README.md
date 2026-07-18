# factory

```go
import "github.com/altessa-s/go-atlas/data/limiters/budget/factory"
```

Package `factory` provides a fluent builder for creating a budget limiter from configuration.
`BudgetLimiterBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
limiter, err := factory.New(cfg.BudgetLimiter).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | In-process map | `UseScheduler` (optional) |
| `redis` | Redis | `UseRedisClient` |
| `nats` | NATS JetStream KV | `UseJetstream` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `BudgetLimiterBuilder` for the given budget limiter config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder |
| `UseRedisClient` | Sets the Redis client for Redis storage backends |
| `UseJetstream` | Sets the NATS JetStream context for NATS storage backends |
| `UseScheduler` | Sets the scheduler for background cleanup task registration |
| `UseCollector` | Sets the Prometheus metrics collector |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the budget limiter |
