# factory

```go
import "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/factory"
```

Package `factory` provides a fluent builder for creating a token-bucket rate limiter from configuration.
`LimiterBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
limiter, err := factory.New(cfg.Limiter).
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
| `New(cfg)` | Creates a `LimiterBuilder` for the given limiter config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis storage backends |
| `UseJetstream` | Sets the NATS JetStream context for NATS storage backends |
| `UseScheduler` | Sets the scheduler for background cleanup task registration |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the rate limiter |
