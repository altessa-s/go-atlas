# cache

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
```

Package `cache` provides a gRPC server interceptor for response caching with compression support. Backed by any `Cacher` implementation (Redis,
freecache, LRU, noop). Supports per-method configuration, decision functions, key generators, metadata processors, and cache headers.

## Key types

| Type / Interface    | Description                                                             |
|---------------------|-------------------------------------------------------------------------|
| `Cacher`            | Alias for `data/cache.Cacher` -- pluggable cache backend               |
| `MethodConfig`      | Per-method caching configuration with response prototype and compression |
| `KeyGenerator`      | Function that generates cache keys from method, request, and metadata   |
| `DecisionFunc`      | Function that decides whether to cache a response and for how long      |
| `Serializer`        | Handles response serialization/deserialization with optional compression |
| `MetadataProcessor` | Custom function to process gRPC metadata for cache key hashing          |

## Options

| Option                  | Default                        | Description                                      |
|-------------------------|--------------------------------|--------------------------------------------------|
| `WithMethod`            | --                             | Register a method with response prototype         |
| `WithMethodConfig`      | --                             | Bulk register methods with compression presets    |
| `WithDefaultTTL`        | 5m                             | Default cache TTL (1s to 24h)                     |
| `WithKeyGenerator`      | `DefaultKeyGenerator`          | Custom cache key generation function              |
| `WithCacheDecision`     | success-only with default TTL  | Custom caching decision function                  |
| `WithSerializer`        | JSON                           | Response serialization format                     |
| `WithCompression`       | none                           | Gzip compression with custom size/level settings  |
| `WithCompressionPreset` | none                           | Predefined compression preset (Fast/Balanced/Best) |
| `WithCacheHeaders`      | true                           | Add `x-cache` hit/miss headers to responses       |
| `WithKeysPrefix`        | ""                             | Optional prefix for cache key namespacing         |
| `WithMetadataKeys`      | default keys                   | gRPC metadata keys to include in cache key hash   |
| `WithMetadataProcessor` | nil                            | Custom metadata processing for key generation     |
| `WithIgnoreMethods`     | --                             | Methods to skip caching                           |
| `WithIgnorePatterns`    | reflection, health             | Regex patterns for methods to skip                |
| `WithLogger`            | discard                        | Structured logger                                 |

## Subpackages

| Package                          | Description                                      |
|----------------------------------|--------------------------------------------------|
| [compression](./compression)     | High-performance gzip compression for cache ops  |
