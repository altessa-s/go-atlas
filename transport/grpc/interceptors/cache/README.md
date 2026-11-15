# cache

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
```

Package `cache` provides a gRPC server interceptor for response caching with compression support. Backed by any [`cache.Cacher`](../../../../cache) implementation (Redis,
freecache, LRU, noop). Supports per-method configuration, decision functions, key generators, metadata processors, and cache headers.

## Key types

| Type / Interface    | Description                                                             |
|---------------------|-------------------------------------------------------------------------|
| `cache.Cacher`            | Pluggable backend from `github.com/altessa-s/go-atlas/cache`               |
| `MethodPolicy`      | Per-method caching policy: response prototype and compression            |
| `KeyGenerator`      | Function that generates cache keys from method, request, and metadata   |
| `DecisionFunc`      | Function that decides whether to cache a response and for how long      |
| `Serializer`        | Handles response serialization/deserialization with optional compression |
| `MetadataProcessor` | Custom function to process gRPC metadata for cache key hashing          |

## Options

| Option                  | Default                        | Description                                      |
|-------------------------|--------------------------------|--------------------------------------------------|
| `WithMethod`            | --                             | Register a method with response prototype         |
| `WithMethodPolicy`      | --                             | Bulk register methods with compression presets    |
| `WithDefaultTTL`        | 5m                             | Default cache TTL (1s to 24h)                     |
| `WithKeyGenerator`      | `NewKeyGenerator(nil, nil)`    | Custom cache key generation function              |
| `WithCacheDecision`     | success-only with default TTL  | Custom caching decision function (receives req)   |
| `WithSerializer`        | protobuf (`DefaultSerializer`) | Response serialization format                     |
| `WithCompression`       | none                           | Gzip compression with custom size/level settings  |
| `WithCompressionPreset` | none                           | Predefined compression preset (Fast/Balanced/Best) |
| `WithCacheHeaders`      | true                           | Add `x-cache` hit/miss headers to responses       |
| `WithKeysPrefix`        | ""                             | Optional prefix for cache key namespacing         |
| `WithMetadataKeys`      | default keys                   | gRPC metadata keys to include in cache key hash   |
| `WithMetadataProcessor` | nil                            | Custom metadata processing for key generation     |
| `WithIgnoreMethods`     | --                             | Methods to skip caching                           |
| `WithIgnorePatterns`    | reflection, health             | Regex patterns for methods to skip                |
| `WithLogger`            | discard                        | Structured logger                                 |

## Cache keys

The default generator is built once per interceptor and snapshots `DefaultMetadataKeys` at construction time. Every hashed component (method, request
bytes, metadata keys and values, processor output) is written as tag + length + payload, so distinct inputs cannot collide by concatenation (for example
`tenant-id=["auser-idb"]` versus `tenant-id=["a"], user-id=["b"]`). This framing changed the key format: entries written by earlier versions are no
longer hit and expire on their TTL. When key generation fails the call bypasses the cache entirely.

## Subpackages

| Package                          | Description                                      |
|----------------------------------|--------------------------------------------------|
| [compression](./compression)     | High-performance gzip compression for cache ops  |

Method names and processor-output keys are case-sensitive; protobuf requests use deterministic serialization, including map fields. Keys produced by
older versions may therefore miss and expire normally. Selected metadata is bounded to 100 keys, 256 bytes per key, 20 values per key and 1024 bytes
per value. Processor output uses the same key/value bounds. Exceeding a bound returns `ErrMetadataTooLarge` and bypasses caching, rather than omitting
identity fields from the cache key.

A custom `MetadataProcessor` runs even when the incoming context has no gRPC metadata. It receives an empty writable metadata map in that case, so
identity stored in context values still contributes to cache partitioning.
