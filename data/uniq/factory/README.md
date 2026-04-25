# factory

```go
import "github.com/altessa-s/go-atlas/data/uniq/factory"
```

Package `factory` provides a fluent builder for creating a `uniq.Uniq` from configuration.
`UniqBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
u, err := factory.New(cfg.Cache).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

When config is `nil` or storage type is `memory`, a no-op provider is used.

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | No-op (in-process) | — |
| `redis` | Redis | `UseRedisClient` |
| `nats` | NATS KV | `UseNatsConn` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `UniqBuilder` for the given cache storage config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseRedisClient` | Sets the Redis client for Redis-backed providers |
| `UseNatsConn` | Sets the NATS connection for NATS-backed providers |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the unique value manager |
