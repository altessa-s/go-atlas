# factory

```go
import "github.com/altessa-s/go-atlas/factory/store/mongo"
```

Package `factory` provides a fluent builder for creating a MongoDB cursor storage from configuration. `CursorStorageBuilder` uses deferred error
accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
storage, err := factory.New(cfg.CursorStorage).
    UseLogger(logger).
    UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
    WithTTL(24 * time.Hour).
    Build(ctx)
```

`memory` is built in. `redis` and `nats` use the function registered with `UseStorage`; without one `Build` fails naming the package to add. Any
other type fails `Build` with an unsupported storage type error.

## Supported Storage Types

| Type     | Backend           | Requires                                                                                                       |
|----------|-------------------|----------------------------------------------------------------------------------------------------------------|
| `memory` | In-process map    | `UseScheduler` (optional)                                                                                      |
| `redis`  | Redis             | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/store/mongo/storages/redis`](storages/redis) |
| `nats`   | NATS JetStream KV | `UseStorage("nats", natsfactory.Builder(js))` from [`factory/store/mongo/storages/nats`](storages/nats)         |

## Methods

### Constructor

| Method     | Description                                                                                           |
|------------|-------------------------------------------------------------------------------------------------------|
| `New(cfg)` | Creates a `CursorStorageBuilder` for the given storage backend config (`storageconfig.BackendConfig`) |

### Dependencies

| Method             | Description                                                                                |
|--------------------|--------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                                 |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                        |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`, `nats`) |
| `UseScheduler`     | Sets the scheduler for background cleanup task registration                                |

### Configuration

| Method         | Description                                        |
|----------------|----------------------------------------------------|
| `WithTTL(ttl)` | Sets the TTL applied to all cursor storage entries |

### Terminal

| Method       | Description                              |
|--------------|------------------------------------------|
| `Build(ctx)` | Assembles and returns the cursor storage |

## NATS bucket

The NATS bucket (`storage.nats.bucket`, default `cursor`), its mandatory `WithTTL` key TTL and the checks of an existing bucket are described in
[`factory/store/mongo/storages/nats`](storages/nats).
