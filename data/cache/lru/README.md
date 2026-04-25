# lru

```go
import "github.com/altessa-s/go-atlas/data/cache/lru"
```

Package `lru` provides generic thread-safe LRU cache implementations. Supports standard and sharded modes with singleflight-based `GetOrCompute`
for deduplicating concurrent loads.
