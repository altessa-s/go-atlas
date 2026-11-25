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

Both builders borrow the injected client. Use `SentinelBuilder(client)` for the result of `NewFailoverClusterClient`, including a non-nil
`ManagedClient.SentinelCluster()`. It compares fresh Sentinel discovery to the routed primary before verifying its role, cluster mode and eviction
policy. The default builder requires a standalone primary or complete Cluster discovery. A Ring or decorated client fails topology verification.
