# factory

```go
import "github.com/altessa-s/go-atlas/data/cache/factory"
```

Package `factory` provides a fluent builder for creating cache providers from configuration. `StorageBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
provider, err := factory.New(cfg.Cache).
    UseLogger(logger).
    UseRedisClient(redisClient).
    Build()
```

When config is `nil` or storage type is `memory`, an in-memory FreeCache provider is returned. `redis` builds a Redis provider (its `redis` block is
required). Any other type, including `nats`, fails `Build` with an unsupported storage type error.

## Supported Storage Types

| Type | Backend | Requires |
|------|---------|----------|
| `memory` | FreeCache (in-process) | — |
| `redis` | Redis | `UseRedisClient` |

## Generation stores

`NewGenerationStore(cfg)` builds the `generation.Store` that `cache.WithGeneration` consumes, from the same `CacheStorageConfig`:

```go
store, err := factory.NewGenerationStore(cfg.Cache).
    UseRedisClient(redisClient).
    UseRedisOptions(redisgen.WithMaxStaleness(500 * time.Millisecond)).
    Build()
```

| Type | Store | Requires |
|------|-------|----------|
| `nil` / `memory` | in-process (`generation/storages/memory`), not shared | — |
| `redis` | shared (`generation/storages/redis`), keys under `<keysPrefix>:generation:` | `UseRedisClient` |

Any other type fails `Build`. Options passed to `UseRedisOptions` are applied after the derived key prefix, so an explicit `WithKeyPrefix` wins.

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `StorageBuilder` for the given cache storage config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder |
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseRedisClient` | Sets the Redis client for Redis-backed providers |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the cache provider |
