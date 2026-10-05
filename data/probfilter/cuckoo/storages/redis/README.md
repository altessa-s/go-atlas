# redis

```go
import "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
```

Package `redis` implements Cuckoo filter storage using Redis for distributed probabilistic existence checks with deletion support shared across
multiple service instances. Requires the [RedisBloom](https://redis.io/docs/latest/develop/data-types/probabilistic/cuckoo-filter/) module (`CF.*`
commands).

## Options

| Option           | Default      | Description                           |
|------------------|--------------|---------------------------------------|
| `WithCapacity`   | `100000`     | Maximum number of items the filter holds |
| `WithKeyPrefix`  | `"cuckoo:"`  | Redis key prefix for the filter        |
| `WithExpansion`  | `0`          | RedisBloom `EXPANSION` growth factor; `0` keeps the RedisBloom default |

`Stage` reserves the replacement filter of a rebuild under a private staging key in the live key's cluster hash slot; `Commit` renames it onto the live
key atomically. The staging key carries a TTL, so a process that crashes mid-rebuild cannot leak it.

`Delete` is bound to the filter generation it observed: every rebuild commit attempt advances the filter's generation counter, and a delete request
delayed past a rebuild does nothing instead of removing a colliding member of the rebuilt filter; `Delete` then returns an error ("filter replaced
during delete") and the caller may retry against the new filter.

Rebuilds of the shared filter are serialized across processes by a rebuild lease (`BeginRebuild`): a concurrent rebuild fails with
`probfilter.ErrRebuildInProgress` before reading its source, and a rebuild that lost its lease cannot publish
(`probfilter.ErrRebuildSuperseded`), so an older snapshot never overwrites a newer one.
