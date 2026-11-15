# factory

```go
import redisfactory "github.com/altessa-s/go-atlas/factory/store/mongo/storages/redis"
```

Maps the `redis` block of the storage backend configuration to a [Redis cursor storage](../../../../../store/mongo/storages/redis/) for
[`factory/store/mongo`](../..), which builds only the in-memory storage.

## Usage

```go
storage, err := mongofactory.New(cfg.CursorStorage).
    UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
    WithTTL(24 * time.Hour).
    Build(ctx)
```

| Symbol              | Description                                                                              |
|---------------------|------------------------------------------------------------------------------------------|
| `Builder(client)`   | Storage function for the `redis` type; keys carry `redis.keysPrefix` and the builder TTL |
| `ErrClientRequired` | Returned when the function has no client                                                 |
| `ErrConfigRequired` | Returned when the configuration has no `redis` block                                     |
