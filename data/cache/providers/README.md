# providers

```go
import "github.com/altessa-s/go-atlas/data/cache/providers"
```

Package `providers` defines the cache `Provider` interface and common errors. Implementations live in subpackages and are injected into the
top-level `cache.Cache` to supply the underlying storage backend.

## Key types

| Type / Interface | Description                                                       |
|------------------|-------------------------------------------------------------------|
| `Provider`       | Interface: Save, Get, Delete, DeleteMany, Exists                  |
| `ErrMissing`     | Sentinel error returned when a cache key is not found             |

## Subpackages

| Package                      | Description                        |
|------------------------------|------------------------------------|
| [freecache](./freecache)     | Zero-GC in-memory provider         |
| [lru](./lru)                 | In-memory LRU provider             |
| [noop](./noop)               | No-op provider for testing         |
| [redis](./redis)             | Distributed Redis provider         |
