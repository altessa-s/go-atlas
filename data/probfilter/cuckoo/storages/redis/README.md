# redis

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
```

Package `redis` implements Cuckoo filter storage using Redis for distributed probabilistic existence checks with deletion support shared across
multiple service instances. Requires the [RedisBloom](https://redis.io/docs/latest/develop/data-types/probabilistic/cuckoo-filter/) module (`CF.*` commands).

## Options

| Option           | Default      | Description                           |
|------------------|--------------|---------------------------------------|
| `WithCapacity`   | `100000`     | Maximum number of items the filter holds |
| `WithKeyPrefix`  | `"cuckoo:"`  | Redis key prefix for the filter        |
