# redis

```go
import "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
```

Package `redis` implements Bloom filter storage using Redis for distributed probabilistic existence checks shared across multiple service
instances. Requires the [RedisBloom](https://redis.io/docs/latest/develop/data-types/probabilistic/bloom-filter/) module (`BF.*` commands).

## Options

| Option                  | Default    | Description                           |
|-------------------------|------------|---------------------------------------|
| `WithExpectedItems`     | `100000`   | Expected number of items in the filter |
| `WithFalsePositiveRate` | `0.01`     | Target false-positive probability      |
| `WithKeyPrefix`         | `"bloom:"` | Redis key prefix for the filter        |
