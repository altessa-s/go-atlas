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

Both builders borrow the injected client. Use `SentinelBuilder(client)` for the result of `NewFailoverClusterClient`, including a non-nil
`ManagedClient.SentinelCluster()`. It compares fresh Sentinel discovery to the routed primary before verifying its role, cluster mode and eviction
policy. The default builder requires a standalone primary or complete Cluster discovery. A Ring or decorated client fails topology verification.
