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
    UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

The builder builds only the in-memory storage. `redis` and `nats` use the function registered with `UseStorage`; without one `Build` fails naming
the package to add. Any other type fails `Build` with an unsupported storage type error.

## Supported Storage Types

| Type     | Backend           | Requires                                                                                                             |
|----------|-------------------|----------------------------------------------------------------------------------------------------------------------|
| `memory` | In-process map    | `UseScheduler` (optional)                                                                                            |
| `redis`  | Redis             | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/coord/limiters/storages/redis`](../storages/redis) |
| `nats`   | NATS JetStream KV | `UseStorage("nats", natsfactory.Builder(js))` from [`factory/coord/limiters/storages/nats`](../storages/nats)         |

## Methods

### Constructor

| Method     | Description                                                        |
|------------|--------------------------------------------------------------------|
| `New(cfg)` | Creates a `TokenBucketLimiterBuilder` for the given limiter config |

### Dependencies

| Method             | Description                                                                                |
|--------------------|--------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                                 |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                        |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`, `nats`) |
| `UseScheduler`     | Sets the scheduler for background cleanup task registration                                |
| `UseMeter`         | Sets the metrics collector for recording rate limiter metrics                              |
| `UseClientService` | Sets the client service used to resolve per-client rate limits from authenticated tokens   |

### Terminal

| Method  | Description                            |
|---------|----------------------------------------|
| `Build` | Assembles and returns the rate limiter |
