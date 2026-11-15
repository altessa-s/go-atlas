# factory

```go
import redisfactory "github.com/altessa-s/go-atlas/factory/reliability/saga/storages/redis"
```

Maps the `redis` block of the saga storage configuration to a [Redis saga store](../../../../../reliability/saga/storages/redis/) for
[`factory/reliability/saga`](../..), which builds only the memory and sqldb stores.

## Usage

```go
orch, err := sagafactory.New(&cfg, def).
    UseStorage(sagaconfig.StorageTypeRedis, redisfactory.Builder(rdb)).
    Build(ctx)
```

| Symbol              | Description                                                                                           |
|---------------------|-------------------------------------------------------------------------------------------------------|
| `Builder(client)`   | Storage function for the `redis` type; keys carry `storage.redis.keysPrefix`, TTL `storage.redis.ttl` |
| `ErrClientRequired` | Returned when the function has no client                                                              |
| `ErrConfigRequired` | Returned when the configuration has no `storage.redis` block                                          |
