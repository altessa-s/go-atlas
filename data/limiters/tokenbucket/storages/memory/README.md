# memory

```go
import "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages/memory"
```

Package `memory` provides an in-memory token bucket storage backend. Suitable for single-instance deployments, tests, and local development.
Includes automatic cleanup of expired buckets.
