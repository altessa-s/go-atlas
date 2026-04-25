# factory

```go
import "github.com/altessa-s/go-atlas/data/cache/factory"
```

Package `factory` provides a fluent builder for creating cache providers from configuration.
`ProviderBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
provider, err := factory.New(cfg.Cache).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

When config is `nil` or storage type is `memory`, an in-memory FreeCache provider is returned.

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | FreeCache (in-process) | — |
| `redis` | Redis | `UseRedisClient` |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ProviderBuilder` for the given cache storage config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder |
| `UseRedisClient` | Sets the Redis client for Redis-backed providers |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the cache provider |
