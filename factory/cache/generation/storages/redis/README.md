# factory

```go
import redisgenfactory "github.com/altessa-s/go-atlas/factory/cache/generation/storages/redis"
```

Maps the `redis` block of the cache storage configuration to a shared [Redis generation store](../../../../../cache/generation/storages/redis/)
for [`factory/cache`](../../..), which builds only the in-process store. Keys live under `<keysPrefix>:generation:`.

## Usage

```go
store, err := cachefactory.NewGenerationStore(&cfg.Cache).
    UseStore(storageconfig.CacheStorageTypeRedis, redisgenfactory.Builder(redisClient, redisgen.WithMaxStaleness(500*time.Millisecond))).
    Build()
```

| Symbol                     | Description                                                                                                                    |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `Builder(client, opts...)` | Generation store function for the `redis` type; `opts` apply after the derived key prefix, so an explicit `WithKeyPrefix` wins |
| `ErrClientRequired`        | Returned when the function has no client                                                                                       |
| `ErrConfigRequired`        | Returned when the configuration has no `redis` block                                                                           |
