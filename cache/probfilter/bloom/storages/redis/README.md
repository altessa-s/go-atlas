# redis

```go
import "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
```

Package `redis` implements Bloom filter storage using Redis for distributed probabilistic existence checks shared across multiple service instances.
Requires the [RedisBloom](https://redis.io/docs/latest/develop/data-types/probabilistic/bloom-filter/) module (`BF.*` commands).

## Options

| Option                            | Default    | Description                                                                          |
|-----------------------------------|------------|--------------------------------------------------------------------------------------|
| `WithExpectedItems`               | `100000`   | Expected number of items in the filter                                               |
| `WithFalsePositiveRate`           | `0.01`     | Target false-positive probability                                                    |
| `WithKeyPrefix`                   | `"bloom:"` | Redis key prefix for the filter                                                      |
| `WithInsecureTreatMissingAsEmpty` | off        | A vanished filter answers "absent" instead of failing with `probfilter.ErrFilterLost` |

Rebuilds of the shared filter are serialized across processes by a rebuild lease (`BeginRebuild`): a concurrent rebuild fails with
`probfilter.ErrRebuildInProgress` before reading its source, and a rebuild that lost its lease cannot publish (`probfilter.ErrRebuildSuperseded`), so
an
older snapshot never overwrites a newer one.

`CheckEvictionPolicy` verifies the eviction policy of a standalone primary or every primary identified by `CLUSTER SHARDS`. An `allkeys-*` policy
returns `probfilter.ErrUnsafeEvictionPolicy`. Incomplete topology returns `probfilter.ErrUnverifiableTopology`, including raw Rings and decorated
clients. Only a denied `CONFIG GET` on verified primaries returns `verified == false` without an error. Use `CheckSentinelEvictionPolicy` for a client
created by `NewFailoverClusterClient`.
