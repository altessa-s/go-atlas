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
user, err := cache.GetWithFallbackT(ctx, c, "user:123", func(work context.Context) (User, time.Duration, error) {
    user, err := db.GetUser(work, 123)
    return user, cache.TTLUseDefault, err
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

The fallback receives the shared context; use it for database and network calls. `WithFallbackTimeout` sets its deadline (15 seconds by default).
A new distinct key at `WithMaxConcurrentFallbacks` capacity returns `ErrOverloaded` immediately. Existing keys can still join their running fallback.
Capacity remains occupied until the callback exits, even after every waiter cancels. There is no background admission queue.

## Options

| Option             | Default | Description                                                      |
|--------------------|---------|------------------------------------------------------------------|
| `WithTTL`          | 1h      | Default time-to-live for cache items                             |
| `WithSerializer`   | JSON    | Serialization format                                             |
| `WithKeyNamespace` | none    | Per-context namespace for tenant/subject isolation (see below)   |
| `WithMaxConcurrentFallbacks` | 1024 | Maximum distinct active fallbacks; non-positive disables the limit |
| `WithFallbackTimeout` | 15s | Shared fallback and write deadline; non-positive disables the deadline |
| `WithNegativeTTL` | 0 | Cache missing results; zero disables negative caching |
| `WithCollector` | no-op | Metrics collector |
| `WithName` | `""` | Cache name used in metric labels |
| `WithFailOpen(ioTimeout)` | off | Storage and decode failures become misses; positive `ioTimeout` bounds each storage call (see below) |
| `WithIsolatedResults()` | off | Collapsed callers share serialized bytes and each decodes its own copy (see below) |

## Fail-open mode

`WithFailOpen(ioTimeout)` makes the cache an optimization that never fails a request on its own. A positive `ioTimeout` bounds every
storage `Get`, `GetMany` and `Save` call; zero adds no bound beyond the caller's context.

| Situation | Default | Fail-open |
|-----------|---------|-----------|
| Storage read error or timeout | error returned | miss: `GetWithFallback*` runs the fallback, `Get` returns `ErrMissing`, `GetManyRaw` omits the keys |
| Stored value fails to decode | error returned | miss, and the key is deleted best-effort |
| Fallback rejected with `ErrOverloaded` | `ErrOverloaded` | the fallback runs directly in the caller, unshared, within `WithFallbackTimeout` |
| Write after a successful fallback fails | error returned | the loaded value is returned |
| `Save` fails or times out | error returned | `nil` |
| Fallback error, `ErrMissing`, negative caching | unchanged | unchanged |

The caller's own context cancellation is still returned. Every degraded event increments `cache_errors_total` (except overload) and
`cache_fail_open_total{cache_name, op}`, where `op` is `get`, `decode`, `save` or `overload`.

## Isolated results

By default the callers collapsed onto one `GetWithFallback` / `GetWithFallbackT` fallback receive the value the fallback returned, so a slice,
map or pointer inside it is shared and one caller's mutation is visible to the others. `WithIsolatedResults()` shares the serialized bytes
instead: every caller, including the one that ran the fallback, decodes its own copy. It costs one decode per caller on the miss path.

## Batch reads

```go
raw, err := c.GetManyRaw(ctx, []string{"user:1", "user:2"})          // map[string][]byte of found keys
users, err := cache.GetManyT[User](ctx, c, []string{"user:1", "user:2"}) // each hit decoded into its own User
```

Results are keyed by the caller's keys (namespacing applies to storage keys only); missing and negatively cached keys are absent. The read is a
single `storages.MultiGetter.GetMany` call when the storage implements it, otherwise a loop of `Get`. Hits, misses and negative hits are counted
per key. In `GetManyT` a value that fails to decode returns the error, or with fail-open is a miss for that key and is deleted best-effort.

## Tenant isolation (multi-tenant deployments)

The cache key is the **only** isolation boundary: it is used both for the backend store and for singleflight de-duplication.
If two callers build the **same** bare key, they share the same cache entry **and** can collapse onto the same in-flight
`GetWithFallback` — so one caller may receive another's freshly computed value. In a multi-tenant service this is a
cross-tenant data leak.

**Contract:** in a multi-tenant deployment, keys must be scoped to the tenant/subject. Either include the tenant in every key
you pass, or configure `WithKeyNamespace` so the cache prefixes every operation automatically:

```go
c := cache.New(provider, cache.WithKeyNamespace(func(ctx context.Context) string {
    return principal.FromContext(ctx).Tenant()
}))
```

`WithKeyNamespace` is applied uniformly to `Get`, `GetManyRaw`, `GetManyT`, `Save`, `Exists`, `Delete`, `DeleteMany`, and both the backend and
singleflight key inside `GetWithFallback`, so a `Get` and its matching `Save` always agree. With a namespace function configured, keys are encoded as
`<namespace byte length>:<namespace>:<key>`. An empty namespace is encoded too. Without a namespace function, keys are unchanged.

## Migrating callers

Fallbacks now take `context.Context` as their first argument, including `GetWithFallbackT` callbacks. Pass that context into I/O operations.
Handle `ErrOverloaded` through the application's overload policy. Nil fallback results return `ErrMissing` and can be negatively cached.

The namespace encoding replaces the ambiguous `namespace:key` format. Existing namespaced entries become cold and expire under their old TTL;
new code does not read them. Deploy all callers of a shared namespace together or use separate storage prefixes during a rolling migration.

## Subpackages

| Package                                    | Description                          |
|--------------------------------------------|--------------------------------------|
| [factory](./factory)                       | Configuration-based cache creation   |
| [lru](./lru)                               | Generic thread-safe LRU cache        |
| [providers/freecache](./storages/freecache) | Zero-GC in-memory provider         |
| [providers/lru](./storages/lru)           | In-memory LRU provider               |
| [providers/noop](./storages/noop)         | No-op provider for testing           |
| [providers/redis](./storages/redis)       | Distributed Redis provider           |
