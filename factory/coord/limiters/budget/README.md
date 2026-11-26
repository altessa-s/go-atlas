# factory

```go
import "github.com/altessa-s/go-atlas/factory/coord/limiters/budget"
```

Package `factory` provides a fluent builder for creating a budget limiter from configuration. `BudgetLimiterBuilder` uses deferred error accumulation —
errors from any step are collected and returned at `Build(ctx)` time.

## Quick Start

```go
limiter, err := factory.New(cfg.BudgetLimiter).
    UseLogger(logger).
    UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

The builder builds only the in-memory storage. `redis` and `nats` use the function registered with `UseStorage`; without one `Build` fails naming
the package to add. An unregistered custom type fails with `ErrUnsupported`.

Register a custom backend name with `UseStorage`. Before standalone validation or config loading, call
`cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())`.

Memory storage refuses new keys when the key cap is full of live windows. The limiter setting `insecureEvictLRU: true` allows live-window
eviction, which can let key churn reset another key's quota.

## Supported Storage Types

| Type     | Backend           | Requires                                                                                                             |
|----------|-------------------|----------------------------------------------------------------------------------------------------------------------|
| `memory` | In-process map    | `UseScheduler` (optional)                                                                                            |
| `redis`  | Redis             | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/coord/limiters/storages/redis`](../storages/redis) |
| `nats`   | NATS JetStream KV | `UseStorage("nats", natsfactory.Builder(js))` from [`factory/coord/limiters/storages/nats`](../storages/nats)         |

## Methods

### Constructor

| Method     | Description                                                          |
|------------|----------------------------------------------------------------------|
| `New(cfg)` | Creates a `BudgetLimiterBuilder` for the given budget limiter config |

### Dependencies

| Method             | Description                                                                                |
|--------------------|--------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder                                                            |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                        |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`, `nats`) |
| `UseScheduler`     | Sets the scheduler for background cleanup task registration                                |
| `UseMeter`         | Sets the Prometheus metrics collector                                                      |

### Terminal

| Method  | Description                              |
|---------|------------------------------------------|
| `Build` | Assembles and returns the budget limiter |
