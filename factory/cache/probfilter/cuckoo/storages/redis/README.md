# factory

```go
import cuckooredisfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter/cuckoo/storages/redis"
```

Maps a Cuckoo filter with the `redis` storage type to a [Redis Cuckoo storage](../../../../../../cache/probfilter/cuckoo/storages/redis/) for
[`factory/cache/probfilter`](../../..), which builds only the in-memory storages.

## Usage

```go
manager, err := probfilterfactory.NewManager(cfg.ProbabilisticFilter).
    UseCuckooStorage(probfilterconfig.StorageTypeRedis, cuckooredisfactory.Builder(redisClient)).
    Build(ctx)
```

| Symbol              | Description                                                                                                                              |
|---------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `Builder(client)`   | Cuckoo storage function for the `redis` type; keys carry `storage.redis.keysPrefix`, `capacityMultiplier` maps to RedisBloom `EXPANSION` |
| `ErrClientRequired` | Returned when the function has no client                                                                                                 |
