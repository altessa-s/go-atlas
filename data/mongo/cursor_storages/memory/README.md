# memory

```go
import "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
```

Package `memory` provides an in-memory cursor storage for MongoDB pagination state. Suitable for single-instance deployments and testing where
cursor persistence across restarts is not required.
