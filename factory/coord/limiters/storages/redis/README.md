# factory

```go
import redisfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages/redis"
```

Maps the `redis` block of the limiter storage configuration to a [Redis limiter storage](../../../../../coord/limiters/storages/redis/) for
[`factory/coord/limiters/budget`](../../budget) and [`factory/coord/limiters/tokenbucket`](../../tokenbucket), which build only the in-memory storage.

## Usage

```go
limiter, err := tokenbucketfactory.New(cfg.TokenBucketLimiter).
    UseStorage(limiterconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

| Symbol              | Description                                                                                   |
|---------------------|-----------------------------------------------------------------------------------------------|
| `Builder(client)`   | Storage function for the `redis` type; keys carry `redis.keysPrefix` (default `"ratelimit:"`) |
| `ErrClientRequired` | Returned when the function has no client                                                      |
| `ErrConfigRequired` | Returned when the storage configuration has no `redis` block                                  |
