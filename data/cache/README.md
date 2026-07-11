# cache

```go
import "github.com/altessa-s/go-atlas/data/cache"
```

Package `cache` provides a unified caching interface with multiple backends and serialization support. Features automatic fallback with singleflight
deduplication (prevents cache stampede), configurable TTL, and pluggable serializers (JSON, MessagePack, Protobuf).

## Options

| Option             | Default | Description                                                      |
|--------------------|---------|------------------------------------------------------------------|
| `WithTtl`          | 1h      | Default time-to-live for cache items                             |
| `WithSerializer`   | JSON    | Serialization format                                             |
| `WithKeyNamespace` | none    | Per-context namespace for tenant/subject isolation (see below)   |

## Tenant isolation (multi-tenant deployments)

The cache key is the **only** isolation boundary: it is used both for the backend store and for singleflight de-duplication.
If two callers build the **same** bare key, they share the same cache entry **and** can collapse onto the same in-flight
`GetWithFallback` — so one caller may receive another's freshly computed value. In a multi-tenant service this is a
cross-tenant data leak.

**Contract:** in a multi-tenant deployment, keys must be scoped to the tenant/subject. Either include the tenant in every key
you pass, or configure `WithKeyNamespace` so the cache prefixes every operation automatically:

```go
c := cache.New(provider, cache.WithKeyNamespace(func(ctx context.Context) string {
    return principal.FromContext(ctx).Tenant() // "" disables prefixing (backward-compatible default)
}))
```

`WithKeyNamespace` is applied uniformly to `Get`, `Save`, `Exists`, `Delete`, `DeleteMany`, and both the backend and
singleflight key inside `GetWithFallback`, so a `Get` and its matching `Save` always agree. Returning `""` leaves keys
unchanged, so existing single-tenant callers are unaffected.

## Subpackages

| Package                                    | Description                          |
|--------------------------------------------|--------------------------------------|
| [factory](./factory)                       | Configuration-based cache creation   |
| [lru](./lru)                               | Generic thread-safe LRU cache        |
| [providers/freecache](./providers/freecache) | Zero-GC in-memory provider         |
| [providers/lru](./providers/lru)           | In-memory LRU provider               |
| [providers/noop](./providers/noop)         | No-op provider for testing           |
| [providers/redis](./providers/redis)       | Distributed Redis provider           |
