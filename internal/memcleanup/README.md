# memcleanup

```go
import "github.com/altessa-s/go-atlas/internal/memcleanup"
```

Generic two-phase TTL sweep for RWMutex-guarded in-memory maps and the scheduler wiring of a cleanup cycle. Shared by the memory-backed storages under
`data/` (idempotency keys, MongoDB cursors, rate-limit buckets) that periodically evict expired entries without blocking readers for the duration of a
full scan.

## Key types

| Symbol                     | Description                                                                    |
|----------------------------|--------------------------------------------------------------------------------|
| `Sweep(mu, entries, expired)` | Collect expired keys under `RLock`, delete under `Lock` with expiry re-check |
| `Register(ctx, registrar, guard, cfg)` | Register the storage's `cycle.Guard` entry point as the cleanup task; mark it managed on success |
| `ErrNoScheduler`, `ErrNoSchedule` | Returned by `Register` for a nil registrar or an empty schedule |

## Semantics

Phase 1 scans the whole map under the read lock and collects keys whose `expired(value)` predicate reports true. Phase 2 takes the write lock and
deletes only entries that are still present and still expired — an entry refreshed (TTL re-armed) between the two phases survives instead of being
deleted with the stale phase-1 verdict. The predicate runs with the lock held: keep it fast, never touch the mutex, never mutate the map.

## Usage

```go
now := time.Now()
memcleanup.Sweep(&s.mu, s.entries, func(e *entry) bool {
    return now.After(e.expiresAt)
})
```

Wire a cleanup cycle to a scheduler; once registered, the storage's manual `RunCleanup` (through `guard.Run`) returns
`cycle.ErrManaged` and overlapping runs collapse into one:

```go
s.cleanupTask = cycle.New(s.runCleanupCycle)
err := memcleanup.Register(ctx, s.scheduler, s.cleanupTask, schedtask.Config{
    ID:       "my-memory-cleanup",
    Schedule: schedule,
})
```
