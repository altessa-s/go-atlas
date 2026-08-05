# lru

```go
import "github.com/altessa-s/go-atlas/data/cache/lru"
```

Package `lru` provides generic thread-safe LRU cache implementations. Supports standard, sharded, and expirable modes with singleflight-based
`GetOrCompute` for deduplicating concurrent loads.

## Key types

| Type             | Constructor                        | Use when                                                             |
|------------------|------------------------------------|----------------------------------------------------------------------|
| `Cache`          | `NewCache(size)`                   | Plain size-bounded LRU                                                |
| `ShardedCache`   | `NewShardedCache(size, opts...)`   | Lock contention is the bottleneck; trades memory for parallel access  |
| `ExpirableCache` | `NewExpirableCache(size, ttl)`     | Entries stop being true with age, not just take up room               |
| `Cacher`         | --                                 | The interface all three satisfy                                       |

## Bounding staleness

`Cache` evicts only when full, so a key written once and read forever never refreshes. `ExpirableCache` also bounds *age*: the TTL starts at
insertion and reads do not extend it, so a hot key still expires on schedule. Reach for it when a stale entry is wrong rather than merely
wasteful — a cached authorization decision, a resolved credential.

```go
cache := lru.NewExpirableCache[string, Decision](1000, 5*time.Minute)
```

A size of zero means unbounded and a TTL of zero disables expiry; passing both as zero builds a cache that never releases anything.
