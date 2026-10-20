# memory

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages/memory"
```

Package `memory` provides an in-memory sliding-window rate limit storage backend for the tokenbucket limiter. Suitable for single-instance deployments,
tests, and local development. Idle buckets are removed by `RunCleanup`, called manually or scheduled via `WithScheduler` and `WithCleanupSchedule`.

## Options

| Option                | Default                           | Description                                               |
|-----------------------|-----------------------------------|-----------------------------------------------------------|
| `WithMaxBuckets`      | `DefaultMaxBuckets`               | Global cap on tracked keys; see [Capacity](#capacity)     |
| `WithEviction`        | `DefaultEviction` (`EvictionLRU`) | What a new key gets at the cap once no window has expired |
| `WithMaxIdleTime`     | `DefaultMaxIdleTime`              | Idle time after which cleanup removes a bucket            |
| `WithShards`          | `DefaultShards` (1)               | Independently locked key shards; see below                |
| `WithCleanupSchedule` | —                                 | Cron schedule for cleanup when a scheduler is set         |
| `WithScheduler`       | —                                 | Task registrar that runs cleanup                          |

## Capacity

A new key at `MaxBuckets` first reclaims every window that has expired: its requests have all left the window, so dropping it loses nothing. The
provider tracks a lower bound of the earliest expiry and sweeps only once it has passed, so a full provider with only live windows pays no sweep.
When nothing has expired, `WithEviction` decides:

| Policy           | New key at the cap                              | Trade-off                                                                        |
|------------------|-------------------------------------------------|----------------------------------------------------------------------------------|
| `EvictionLRU`    | Evicts the least recently used live window      | Never refuses; a stream of new keys can push out a busy key and reset its limit  |
| `EvictionReject` | Fails with `ErrCapacityExceeded`, nil LimitInfo | No key loses its limit to churn; new keys wait until a window expires or cleanup |

Tracked keys keep working under both policies. `ErrCapacityExceeded` is not `ErrLimitExceeded`, so the token-bucket and budget limiters count it as a
storage error rather than a rate-limit rejection. In reject mode a refused new key costs one shard lock, not the all-shard lock.

## Multi-key admission

`Provider` implements `storages.MultiStorage`. `AllowAll` locks the shards of its keys in ascending index order, so overlapping calls cannot deadlock,
checks every window and takes a slot on each only when all admit. New keys that find too few free slots lock every shard and reclaim expired windows;
then `EvictionReject` fails the whole call with a `*storages.KeyError` wrapping `ErrCapacityExceeded`, and `EvictionLRU` evicts least recently used
buckets outside the call. A call with more keys than `MaxBuckets` fails the same way. The `KeyError` names the first new key left without room. New
keys reserve their slots in one step, so two calls racing for the last slots never both fail when either fits. A refused or failed call takes no slot
and inserts no bucket.

## Choosing `WithShards`

The cap and least-recently-used eviction are exact and global for every shard count. Sharding lets `Allow` calls on keys in different shards run in
parallel, for existing keys and for new keys while the provider is below `MaxBuckets`; keys in the same shard still contend, and a new key at the cap
locks every shard to find the global victim. Values below 1 mean 1.

| Workload                                                         | Shards |
|------------------------------------------------------------------|--------|
| Usually below `MaxBuckets` (idle cleanup keeps up), many cores   | 8–16   |
| Usually full, new keys keep arriving (huge key space, key churn) | 1      |

On a 14-core machine, parallel hits on existing keys cost ~300 ns with 1 shard, ~140 ns with 8 and ~100 ns with 16; with the provider full and 1 new key
in 10 requests, 1 shard is fastest and 16 shards are ~50% slower.
