# factory

```go
import "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
```

Package `factory` provides a fluent builder for creating an idempotency keeper from configuration. `KeeperBuilder` uses deferred error accumulation —
errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
keeper, err := factory.New(cfg.Idempotency).
    UseLogger(logger).
    UseStorage(idempotencyconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    Build()
```

## Supported Storage Types

The builder builds only the `memory` storage. `redis`, `nats` and `mongo` use the function registered with `UseStorage`; without one `Build`
fails naming the package to add. Any other type fails `Build` with an unsupported storage type error.

| Type     | Backend                                            | Requires                                                                                                             |
|----------|----------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `memory` | In-process map with TTL                            | `UseScheduler` (optional)                                                                                            |
| `redis`  | Redis                                              | `UseStorage("redis", redisfactory.Builder(client))` from [`factory/reliability/idempotency/storages/redis`](storages/redis) |
| `nats`   | NATS JetStream KV                                  | `UseStorage("nats", natsfactory.Builder(js))` from [`factory/reliability/idempotency/storages/nats`](storages/nats)         |
| `mongo`  | MongoDB collection, joins the caller's transaction | `UseStorage("mongo", mongofactory.Builder(db))` from [`factory/reliability/idempotency/storages/mongo`](storages/mongo)     |

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `KeeperBuilder` for the given idempotency config |

### Dependencies

| Method             | Description                                                                                         |
|--------------------|-----------------------------------------------------------------------------------------------------|
| `UseLogger`        | Sets the logger for the builder and all created components                                          |
| `UseDefaultLogger` | Sets the logger to `slog.Default()`                                                                 |
| `UseStorage`       | Registers the `StorageFunc` of a storage type the builder does not build (`redis`, `nats`, `mongo`) |
| `UseScheduler`     | Sets the scheduler for background cleanup task registration                                         |
| `UseMeter`         | Sets the metrics collector passed to the keeper                                                     |

### Caller binding

| Method | Description |
|--------|-------------|
| `UseOwner` | Sets the `idempotency.OwnerFunc` labeling each lock with its caller (default: [`reliability/idempotency/principalowner.PrincipalOwner`](../../../reliability/idempotency/principalowner)) |
| `UseKeyNamespace` | Sets the `idempotency.KeyNamespaceFunc` isolating keys per context (default: the empty namespace) |
| `UseReplayAuthorizer` | Sets the `idempotency.ReplayAuthorizer` deciding every replay from the recorded owner label |

The keeper binds every record to the verified principal of the request (tenant and subject from `auth/principal` via `reliability/idempotency/principalowner`): another caller
presenting the key is refused the record (`idempotency.ErrOwnerMismatch`). A request without a principal records no owner, and the HTTP
middleware and gRPC interceptor never replay such a record or echo its entity ID — they answer "already used". Pass `UseOwner` to bind
records to another identity.

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the idempotency keeper |
