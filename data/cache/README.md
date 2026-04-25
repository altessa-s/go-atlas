# cache

```go
import "github.com/altessa-s/go-atlas/data/cache"
```

Package `cache` provides a unified caching interface with multiple backends and serialization support. Features automatic fallback with singleflight
deduplication (prevents cache stampede), configurable TTL, and pluggable serializers (JSON, MessagePack, Protobuf).

## Options

| Option           | Default | Description                          |
|------------------|---------|--------------------------------------|
| `WithTtl`        | 1h      | Default time-to-live for cache items |
| `WithSerializer` | JSON    | Serialization format                 |

## Subpackages

| Package                                    | Description                          |
|--------------------------------------------|--------------------------------------|
| [factory](./factory)                       | Configuration-based cache creation   |
| [lru](./lru)                               | Generic thread-safe LRU cache        |
| [providers/freecache](./providers/freecache) | Zero-GC in-memory provider         |
| [providers/lru](./providers/lru)           | In-memory LRU provider               |
| [providers/noop](./providers/noop)         | No-op provider for testing           |
| [providers/redis](./providers/redis)       | Distributed Redis provider           |
