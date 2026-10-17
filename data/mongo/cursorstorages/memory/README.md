# memory

```go
import "github.com/altessa-s/go-atlas/data/mongo/cursorstorages/memory"
```

Package `memory` provides an in-memory cursor storage for MongoDB pagination state. Suitable for single-instance deployments and testing where cursor
persistence across restarts is not required.

`New(ttl, opts...)` falls back to `DefaultTTL` (1h) when `ttl` is not positive. Expired cursors are removed by `RunCleanup`, called manually or
scheduled via `WithScheduler` and `WithCleanupSchedule`.
