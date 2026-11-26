# memory

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
```

Package `memory` provides an in-memory implementation of idempotency storage with configurable TTL. Suitable for single-instance deployments, tests, and
local development.

## Cleanup

With a scheduler and a cleanup schedule, `New` registers the cleanup task
and returns any registration error. Use `WithCleanupTaskID` to give each
storage a distinct task ID when multiple stores share a scheduler.

Without a scheduler or schedule, expiration is checked on key access.
Call `RunCleanup` to remove expired entries for keys that are never accessed again.
