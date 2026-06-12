# freecache

```go
import "github.com/altessa-s/go-atlas/data/cache/providers/freecache"
```

Package `freecache` implements the cache provider interface using FreeCache for zero-GC in-memory caching. Uses off-heap storage to avoid garbage
collection overhead, which matters for high-throughput workloads with large datasets.
