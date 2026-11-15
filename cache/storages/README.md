# storages

```go
import "github.com/altessa-s/go-atlas/cache/storages"
```

Package `storages` defines the cache `Storage` interface and common errors. Implementations live in subpackages and are injected into the top-level
`cache.Cache` to supply the underlying storage backend.

## Key types

| Type / Interface | Description                                                                                                                                                                                           |
|------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Storage`        | Interface: Save, Get, Delete, DeleteMany, Exists                                                                                                                                                      |
| `MultiGetter`    | Optional interface: `GetMany(ctx, keys)` returns found keys only; missing keys are absent. Implemented by every subpackage (Redis uses a pipeline of GETs, not MGET, so Cluster cross-slot keys work) |
| `InProcess`      | Optional interface: `InProcess()` reports a storage served from process memory; the cache then adds no default timeout to a deadline-less context. Implemented by freecache, lru and noop             |
| `ErrMissing`     | Sentinel error returned when a cache key is not found                                                                                                                                                 |

## Subpackages

| Package                  | Description                |
|--------------------------|----------------------------|
| [freecache](./freecache) | Zero-GC in-memory provider |
| [lru](./lru)             | In-memory LRU provider     |
| [noop](./noop)           | No-op provider for testing |
| [redis](./redis)         | Distributed Redis provider |
