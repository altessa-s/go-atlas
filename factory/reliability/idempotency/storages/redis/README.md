# factory

```go
import redisfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency/storages/redis"
```

Maps the `redis` block of the idempotency storage configuration to a [Redis idempotency storage](../../../../../reliability/idempotency/storages/redis/) for
[`factory/reliability/idempotency`](../..), which builds only the in-memory storage.

## Usage

```go
keeper, err := idempotencyfactory.New(cfg.Idempotency).
    UseStorage(idempotencyconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

| Symbol              | Description                                                                                         |
|---------------------|-----------------------------------------------------------------------------------------------------|
| `Builder(client)`   | Storage function for the `redis` type; keys carry `storage.redis.keysPrefix` and expire after `ttl` |
| `ErrClientRequired` | Returned when the function has no client                                                            |
| `ErrConfigRequired` | Returned when the configuration has no `storage.redis` block                                        |
