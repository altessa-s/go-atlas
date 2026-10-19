# lru

```go
import "github.com/altessa-s/go-atlas/data/cache/lru"
```

Package `lru` provides generic thread-safe LRU cache implementations. Supports standard, sharded, and expirable modes with singleflight-based
`GetOrCompute` for deduplicating concurrent loads.

## Key types

| Type             | Constructor                      | Use when                                                             |
|------------------|----------------------------------|----------------------------------------------------------------------|
| `Cache`          | `NewCache(size)`                 | Plain size-bounded LRU                                               |
| `ShardedCache`   | `NewShardedCache(size, opts...)` | Lock contention is the bottleneck; trades memory for parallel access |
| `ExpirableCache` | `NewExpirableCache(size, ttl)`   | Entries stop being true with age, not just take up room              |
| `Cacher`         | --                               | The interface all three satisfy                                      |

## Bounding staleness

`Cache` evicts only when full, so a key written once and read forever never refreshes. `ExpirableCache` also bounds *age*: the TTL starts at insertion
and reads do not extend it, so a hot key still expires on schedule. Reach for it when a stale entry is wrong rather than merely wasteful — a cached
authorization decision, a resolved credential.

```go
cache := lru.NewExpirableCache[string, Decision](1000, 5*time.Minute)
```

A size of zero means unbounded and a TTL of zero disables expiry; passing both as zero builds a cache that never releases anything.

Every entry records its expiry instant and every operation judges expiry against it under the cache's lock, so an entry is absent for all readers and
writers from the instant it expires. There is no background goroutine: writes free the expired entries (a sweep at most once per TTL, and always before
a new key would evict from a full cache, so a live entry is never evicted while an expired one remains; a write replaces an expired entry of its key). A
lower bound of the earliest expiry skips sweeps that could find nothing. `WithExpirableClock` injects the one clock both expiry and cleanup follow, for
tests. `ExpirableCache` also has conditional writes, atomic with respect to every other operation and to expiry: `PutIfAbsent(key, v)` (like
`sync.Map.LoadOrStore`: the existing unexpired value and `true`, or stores `v`) and `CompareAndSwap(key, old, new)` (stores `new` only over an unexpired
entry `== old`; `old` must be comparable). Together they make a once-per-TTL gate without an outer mutex:

```go
last, loaded := attempts.PutIfAbsent(k, now)
if loaded && (now.Sub(last) < cooldown || !attempts.CompareAndSwap(k, last, now)) {
    return false // another caller ran recently, or won the race to run now
}
return true
```

## Concurrent loads

`GetOrCompute` groups calls by Go key equality; values of different dynamic types and distinct pointers remain distinct even when their printed forms
match. Sharding uses the same equality contract. A nil value is a valid cached result when `V` permits it.

Each caller cancels its own wait. The shared callback retains the first caller's context values and receives a 15-second deadline, independent of that
caller's cancellation. Use the callback context for I/O. The number of distinct LRU computations is unrestricted; use
[`singleflight`](../../../core/runtime/singleflight) with an admission limit when managing uncached work directly.
