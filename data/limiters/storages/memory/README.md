# memory

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages/memory"
```

Package `memory` provides an in-memory sliding-window rate limit storage backend for the tokenbucket limiter. Suitable for single-instance
deployments, tests, and local development. Idle buckets are removed by `RunCleanup`, called manually or scheduled via `WithScheduler` and
`WithCleanupSchedule`.

## Options

| Option                | Default               | Description                                                                         |
|-----------------------|-----------------------|-------------------------------------------------------------------------------------|
| `WithMaxBuckets`      | `DefaultMaxBuckets`   | Global cap on tracked keys; a new key at the cap evicts the least recently used one |
| `WithMaxIdleTime`     | `DefaultMaxIdleTime`  | Idle time after which cleanup removes a bucket                                      |
| `WithShards`          | `DefaultShards` (1)   | Independently locked key shards; see below                                          |
| `WithCleanupSchedule` | —                     | Cron schedule for cleanup when a scheduler is set                                   |
| `WithScheduler`       | —                     | Task registrar that runs cleanup                                                    |

## Choosing `WithShards`

The cap and least-recently-used eviction are exact and global for every shard count. Sharding lets `Allow` calls on keys in different
shards run in parallel, for existing keys and for new keys while the provider is below `MaxBuckets`; keys in the same shard still contend,
and a new key at the cap locks every shard to find the global victim. Values below 1 mean 1.

| Workload                                                         | Shards |
|------------------------------------------------------------------|--------|
| Usually below `MaxBuckets` (idle cleanup keeps up), many cores   | 8–16   |
| Usually full, new keys keep arriving (huge key space, key churn) | 1      |

On a 14-core machine, parallel hits on existing keys cost ~300 ns with 1 shard, ~140 ns with 8 and ~100 ns with 16; with the provider full
and 1 new key in 10 requests, 1 shard is fastest and 16 shards are ~50% slower.
