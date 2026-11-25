# freecache

```go
import "github.com/altessa-s/go-atlas/cache/storages/freecache"
```

Package `freecache` implements the cache provider interface using FreeCache for zero-GC in-memory caching. Uses off-heap storage to avoid garbage
collection overhead, which matters for high-throughput workloads with large datasets.

Implements the optional `storages.MultiGetter` batch read (`GetMany`).

FreeCache stores the caller's bytes and limits the combined physical key and value to 1/1024 of its configured size minus the entry header. At 1 MiB, a
one-byte key and the cache facade's one-byte format tag leave 998 serialized bytes. The adapter hashes keys when doing so shortens them.

FreeCache handles expiration. Zero and negative TTLs have no expiry. Positive TTLs round up to whole seconds, so 500 ms becomes one second. A TTL above
`MaxTTL` (3,650 days) returns `ErrTTLOutOfRange` before the write.
