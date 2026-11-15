# redis

```go
import "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
```

Package `redis` implements Bloom filter storage using Redis for distributed probabilistic existence checks shared across multiple service instances.
Requires the [RedisBloom](https://redis.io/docs/latest/develop/data-types/probabilistic/bloom-filter/) module (`BF.*` commands).

## Options

| Option                  | Default    | Description                            |
|-------------------------|------------|----------------------------------------|
| `WithExpectedItems`     | `100000`   | Expected number of items in the filter |
| `WithFalsePositiveRate` | `0.01`     | Target false-positive probability      |
| `WithKeyPrefix`         | `"bloom:"` | Redis key prefix for the filter        |

Rebuilds of the shared filter are serialized across processes by a rebuild lease (`BeginRebuild`): a concurrent rebuild fails with
`probfilter.ErrRebuildInProgress` before reading its source, and a rebuild that lost its lease cannot publish (`probfilter.ErrRebuildSuperseded`), so
an
older snapshot never overwrites a newer one.

`CheckEvictionPolicy` verifies that no Redis master runs an `allkeys-*` `maxmemory-policy`, which could evict the filter and its rebuild metadata
(`probfilter.ErrUnsafeEvictionPolicy`); it reports `verified == false` without an error when the policy cannot be read.
