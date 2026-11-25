# lru

```go
import "github.com/altessa-s/go-atlas/cache/storages/lru"
```

Package `lru` implements the cache provider interface using an in-memory LRU eviction policy. Suitable for single-instance deployments where bounded
memory usage is required.

Sizes below 128 use a single shard, which can hold all requested entries, with a minimum capacity of one. Larger caches may evict from a full shard
while others still have room. See [sharded capacity](../../lru/README.md#sharded-capacity) for the retention guarantee and shard-count option.

Implements the optional `storages.MultiGetter` batch read (`GetMany`).
