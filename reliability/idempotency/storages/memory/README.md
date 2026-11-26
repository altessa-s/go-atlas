# memory

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
```

Package `memory` provides an in-memory implementation of idempotency storage with configurable TTL. Suitable for single-instance deployments, tests, and
local development.

## Cleanup

With a scheduler and cleanup schedule, `New` registers the cleanup task and returns any registration error. Use `WithCleanupTaskID` to assign
each storage a distinct task ID when sharing a scheduler. Two memory stores using the same scheduler and ID get
`ErrDuplicateCleanupTaskID` on the second registration. A scheduled store requires a registrar with comparable identity, such as a pointer.

Without a scheduler or schedule, expiration is checked on key access. Call `RunCleanup` to remove expired entries for keys that are never
accessed again.

## Orphan takeover

`WithClock` accepts a go-atlas-core `clock.Clock` for acquisition, expiry, and orphan takeover. The default is `clock.Real()`.
