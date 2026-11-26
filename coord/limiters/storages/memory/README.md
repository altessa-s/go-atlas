# memory

```go
import "github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
```

Package `memory` provides an in-memory sliding-window rate limit storage backend for single-instance deployments, tests, and local development.
`RunCleanup` removes idle buckets whose windows have expired. Call it manually or schedule it via `WithScheduler` and `WithCleanupSchedule`.

## Options

| Option                | Default                           | Description                                               |
|-----------------------|-----------------------------------|-----------------------------------------------------------|
| `WithMaxBuckets`      | `DefaultMaxBuckets`               | Global cap on tracked keys; see [Capacity](#capacity)     |
| `WithInsecureEvictLRU` | Disabled                         | Allows live-window eviction when the key cap is full      |
| `WithMaxIdleTime`     | `DefaultMaxIdleTime`              | Minimum idle time for cleanup of an expired bucket            |
| `WithShards`          | `DefaultShards` (1)               | Independently locked key shards; see below                |
| `WithCleanupSchedule` | —                                 | Cron schedule for cleanup when a scheduler is set         |
| `WithScheduler`       | —                                 | Task registrar that runs cleanup                          |
| `WithClock`           | `clock.Real()`                    | Window, expiry and idle time (tests: `clock.Fake`)        |

The clock must not go backwards: a window keeps its request timestamps in non-decreasing order. A `clock.Fake` that only advances is fine.

## Capacity

A new key at `MaxBuckets` first reclaims an expired window. Each shard indexes its windows by expiry, so admissions inspect only the first entry
in each shard and remove only as many expired windows as needed. The provider tracks a lower bound on the earliest expiry to skip reclamation
when every window is still live.
When every window is live, the default `EvictionReject` returns `ErrCapacityExceeded` with nil `LimitInfo` for new keys. Tracked keys keep
working. With `WithInsecureEvictLRU()`, a new key evicts the globally least recently used window. Key churn can then reset another key's quota
before its window expires. Both limiter factories expose this opt-in as `insecureEvictLRU: true` in their limiter settings.

The token-bucket and budget limiters report `ErrCapacityExceeded` as a storage error. In reject mode, a new-key refusal ordinarily takes one
shard lock.

## Multi-key admission

`Provider` implements `storages.MultiStorage`. `AllowAll` locks the shards of its keys in ascending index order, so overlapping calls cannot deadlock,
checks every window and takes a slot on each only when all admit. New keys that find too few free slots lock every shard and reclaim expired windows;
then `EvictionReject` fails the whole call with a `*storages.KeyError` wrapping `ErrCapacityExceeded`, and `EvictionLRU` evicts least recently used
buckets outside the call. A call with more keys than `MaxBuckets` fails the same way. The `KeyError` names the first new key left without room. New
keys reserve their slots in one step, so two calls racing for the last slots never both fail when either fits. A refused or failed call takes no slot
and inserts no bucket.

## Choosing `WithShards`

The cap and least-recently-used eviction are exact and global for every shard count. Sharding lets `Allow` calls on keys in different shards run in
parallel, for existing keys and for new keys while the provider is below `MaxBuckets`. Reclaiming an expired window or evicting a live window at
the cap can lock every shard. Values below 1 mean 1.

| Workload                                                         | Shards |
|------------------------------------------------------------------|--------|
| Usually below `MaxBuckets` (idle cleanup keeps up), many cores   | 8–16   |
| Usually full, new keys keep arriving (huge key space, key churn) | 1      |

On a 14-core machine, parallel hits on existing keys cost ~300 ns with 1 shard, ~140 ns with 8 and ~100 ns with 16; with the provider full and 1 new
key
in 10 requests, 1 shard is fastest and 16 shards are ~50% slower.
