# factory

```go
import redisfactory "github.com/altessa-s/go-atlas/factory/cache/storages/redis"
```

Maps the `redis` block of the cache storage configuration to a [Redis cache storage](../../../../cache/storages/redis/) for
[`factory/cache`](../..), which builds only the in-memory storage.

## Usage

```go
storage, err := cachefactory.New(&cfg.Cache).
    UseStorage(storageconfig.CacheStorageTypeRedis, redisfactory.Builder(redisClient)).
    Build()
```

| Symbol              | Description                                                          |
|---------------------|----------------------------------------------------------------------|
| `Builder(client)`   | Storage function for the `redis` type; keys carry `redis.keysPrefix` |
| `ErrClientRequired` | Returned when the function has no client                             |
| `ErrConfigRequired` | Returned when the configuration has no `redis` block                 |
