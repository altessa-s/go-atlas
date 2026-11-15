# factory

```go
import storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
```

Declares the storage function shared by [`factory/coord/limiters/budget`](../budget) and [`factory/coord/limiters/tokenbucket`](../tokenbucket). Both
build only the in-memory storage; a driver storage registers its `StorageFunc` from its own factory package, so one builder serves both limiters.

## Usage

```go
limiter, err := budgetfactory.New(cfg.BudgetLimiter).
    UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

| Symbol        | Description                                                                                       |
|---------------|---------------------------------------------------------------------------------------------------|
| `StorageFunc` | Builds the storage of one type from the storage block and the limiter's `natsContention` settings |

## Storage builders

| Type    | Package                                         |
|---------|-------------------------------------------------|
| `redis` | [`factory/coord/limiters/storages/redis`](redis) |
| `nats`  | [`factory/coord/limiters/storages/nats`](nats)   |
