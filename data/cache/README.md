# cache

```go
import "github.com/altessa-s/go-atlas/data/cache"
```

Package `cache` provides a unified caching interface with multiple backends and serialization support. Features automatic fallback with singleflight
deduplication (prevents cache stampede), configurable TTL, and pluggable serializers (JSON, MessagePack, Protobuf).

## Typed reads

`GetWithFallbackT[T]` is the typed variant of `Cache.GetWithFallback`: the destination is a value of `T`, so the `any`-based
method's assignability panic-assert and reflection copy-out disappear, and the fallback signature is type-checked:

```go
user, err := cache.GetWithFallbackT(ctx, c, "user:123", func() (User, time.Duration, error) {
    return db.GetUser(ctx, 123), cache.TTLUseDefault, nil
})
```

Both variants share the same singleflight keyspace, so typed and untyped callers of one key collapse onto a single fallback.

### Contexts and the collapsed group

Collapsing concurrent callers onto one fallback means one of them wins the group and the rest wait behind it. Two things follow, and the cache
handles both:

- **A waiter leaves on its own context.** Cancel a caller's request and it stops waiting, even though the shared fetch is still running for the
  others.
- **The winner's cancellation does not poison the waiters.** The shared write runs on a context detached from whichever caller happened to win, so
  that caller going away does not fail the fetch for everyone queued behind it.

What the cache cannot reach is the fallback you supply: `Fallback` takes no context, so the closure uses whatever it captured. **Give a fallback that
does I/O a context of its own rather than the request's** — otherwise the winner's cancellation still aborts the work every waiter is relying on.

## Options

| Option             | Default | Description                                                      |
|--------------------|---------|------------------------------------------------------------------|
| `WithTTL`          | 1h      | Default time-to-live for cache items                             |
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
