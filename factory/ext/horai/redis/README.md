# factory

```go
import redisfactory "github.com/altessa-s/go-atlas/factory/ext/horai/redis"
```

Maps the `storage.redis` block of the scheduler configuration to a [Redis task storage][storage] for
[`factory/ext/horai`](..), which builds only the memory and sqldb task storages. With `ensureIndexes` the RediSearch indexes
are created at startup (bounded to 30 s); otherwise the builder performs no I/O and the indexes must exist (`EnsureIndexes` on the storage).

## Usage

```go
sched, err := schedulerfactory.New(&cfg.Scheduler).
    UseStorage(schedulerconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    Build(ctx)
```

| Symbol              | Description                                                          |
|---------------------|----------------------------------------------------------------------|
| `Builder(client)`   | Task storage function for the `redis` type; the caller owns `client` |
| `ErrClientRequired` | Returned when the function has no client                             |
| `ErrConfigRequired` | Returned when the configuration has no `redis` block                 |

[storage]: https://github.com/altessa-s/go-horai/tree/main/storage/redis
