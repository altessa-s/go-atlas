# factory

```go
import "github.com/altessa-s/go-atlas/factory/coord/limiters/tokenbucket"
```

Package `factory` provides a fluent builder for creating a token-bucket rate limiter from configuration. `TokenBucketLimiterBuilder` uses deferred error
accumulation — errors from any step are collected and returned at `Build(ctx)` time.

## Quick Start

```go
limiter, err := factory.New(cfg.TokenBucketLimiter).
    UseLogger(logger).
    UseStorage(limiterconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

The builder builds only the in-memory storage. `redis` uses the function registered with `UseStorage`; without one `Build` fails naming
the package to add. An unregistered custom type fails with `ErrUnsupported`.

Register a custom backend name with `UseStorage`. Before standalone validation or config loading, call
`cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())`.

Memory storage refuses new keys when the key cap is full of live windows. The limiter setting `insecureEvictLRU: true` allows live-window
eviction, which can let key churn reset another key's quota.

## Supported Storage Types

| Type     | Backend        | Requires                                                                                                             |
|----------|----------------|----------------------------------------------------------------------------------------------------------------------|
| `memory` | In-process map | `UseScheduler` (optional)                                                                                            |
| `redis`  | Redis          | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/coord/limiters/storages/redis`](../storages/redis) |

## Methods

### Constructor

| Method     | Description                                                        |
|------------|--------------------------------------------------------------------|
| `New(cfg)` | Creates a `TokenBucketLimiterBuilder` for the given limiter config |

### Dependencies

| Method             | Description                                                                        |
|--------------------|------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                         |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`) |
| `UseScheduler`     | Sets the scheduler for background cleanup task registration                                |
| `UseMeter`         | Sets the metrics collector for recording rate limiter metrics                              |
| `UseClientService` | Sets the client service used to resolve per-client rate limits from authenticated tokens   |

### Terminal

| Method  | Description                            |
|---------|----------------------------------------|
| `Build` | Assembles and returns the rate limiter |
