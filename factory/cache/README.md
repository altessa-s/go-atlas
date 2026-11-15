# factory

```go
import "github.com/altessa-s/go-atlas/factory/cache"
```

Package `factory` provides a fluent builder for creating cache providers from configuration. `StorageBuilder` uses deferred error accumulation — errors
from any step are collected and returned at `Build()` time.

## Quick Start

```go
provider, err := factory.New(cfg.Cache).
    UseLogger(logger).
    UseStorage(storageconfig.CacheStorageTypeRedis, redisfactory.Builder(redisClient)).
    Build()
```

When config is `nil` or storage type is `memory`, an in-memory FreeCache provider is returned. `redis` uses the function registered with
`UseStorage`; without one `Build` fails naming the package to add. Any other type, including `nats`, fails `Build` with an unsupported storage
type error.

## Supported Storage Types

| Type     | Backend                | Requires                                                                                                       |
|----------|------------------------|----------------------------------------------------------------------------------------------------------------|
| `memory` | FreeCache (in-process) | —                                                                                                              |
| `redis`  | Redis                  | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/cache/storages/redis`](storages/redis) |

## Generation stores

`NewGenerationStore(cfg)` builds the `generation.Storage` that `cache.WithGeneration` consumes, from the same `CacheStorageConfig`:

```go
store, err := factory.NewGenerationStore(cfg.Cache).
    UseStore(storageconfig.CacheStorageTypeRedis,
        redisgenfactory.Builder(redisClient, redisgen.WithMaxStaleness(500*time.Millisecond))).
    Build()
```

| Type             | Store                                                                       | Requires                                                                                                                                       |
|------------------|-----------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `nil` / `memory` | in-process (`generation/storages/memory`), not shared                       | —                                                                                                                                              |
| `redis`          | shared (`generation/storages/redis`), keys under `<keysPrefix>:generation:` | `UseStore("redis", redisgenfactory.Builder(client, opts...))` from [`factory/cache/generation/storages/redis`](generation/storages/redis) |

Any other type fails `Build`. Options passed to `redisgenfactory.Builder` are applied after the derived key prefix, so an explicit `WithKeyPrefix`
wins.

## Methods

### Constructor

| Method     | Description                                                   |
|------------|---------------------------------------------------------------|
| `New(cfg)` | Creates a `StorageBuilder` for the given cache storage config |

### Dependencies

| Method             | Description                                                                        |
|--------------------|------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder                                                    |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`) |

### Terminal

| Method  | Description                              |
|---------|------------------------------------------|
| `Build` | Assembles and returns the cache provider |
