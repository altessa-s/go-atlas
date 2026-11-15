# factory

```go
import bloomredisfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter/bloom/storages/redis"
```

Maps a Bloom filter with the `redis` storage type to a [Redis Bloom storage](../../../../../../cache/probfilter/bloom/storages/redis/) for
[`factory/cache/probfilter`](../../..), which builds only the in-memory storages.

## Usage

```go
manager, err := probfilterfactory.NewManager(cfg.ProbabilisticFilter).
    UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
    Build(ctx)
```

| Symbol              | Description                                                                                                                         |
|---------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `Builder(client)`   | Bloom storage function for the `redis` type; keys carry `storage.redis.keysPrefix`, an `allkeys-*` `maxmemory-policy` fails `Build` |
| `ErrClientRequired` | Returned when the function has no client                                                                                            |
