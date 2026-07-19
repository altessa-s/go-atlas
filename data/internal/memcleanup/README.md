# memcleanup

```go
import "github.com/altessa-s/go-atlas/data/internal/memcleanup"
```

Generic two-phase TTL sweep for RWMutex-guarded in-memory maps. Shared by the memory-backed storages under `data/` (idempotency keys,
MongoDB cursors) that periodically evict expired entries without blocking readers for the duration of a full scan.

## Key types

| Symbol                     | Description                                                                    |
|----------------------------|--------------------------------------------------------------------------------|
| `Sweep(mu, entries, expired)` | Collect expired keys under `RLock`, delete under `Lock` with expiry re-check |

## Semantics

Phase 1 scans the whole map under the read lock and collects keys whose `expired(value)` predicate reports true. Phase 2 takes the write
lock and deletes only entries that are still present and still expired — an entry refreshed (TTL re-armed) between the two phases
survives instead of being deleted with the stale phase-1 verdict. The predicate runs with the lock held: keep it fast, never touch the
mutex, never mutate the map.

## Usage

```go
now := time.Now()
memcleanup.Sweep(&s.mu, s.entries, func(e *entry) bool {
    return now.After(e.expiresAt)
})
```
