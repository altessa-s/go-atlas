# generation

```go
import "github.com/altessa-s/go-atlas/data/cache/generation"
```

Package `generation` defines the cache generation `Store`: a map from a namespace to a number that only grows. A cache that embeds the generation in
its keys is invalidated as a whole by one `Bump`; a store shared by several replicas invalidates the caches of all of them.

```go
type Store interface {
    Current(ctx context.Context, namespace string) (uint64, error)
    Bump(ctx context.Context, namespace string) (uint64, error)
}
```

## Contract

- **Monotonic.** For one namespace a returned value is never smaller than an earlier one. After the backend lost its data this assumes that a fresh
  namespace is seeded from a clock in Unix microseconds (`Seed`; the Redis store uses the server clock), never zero, and that this clock is ahead
  of every generation used before the loss. A backend that restores an older value (a failover to a lagging replica) can fail closed only where it
  knows a newer one; the Redis store documents its residual regression window in [its README](./storages/redis/README.md#semantics).
- **Bounded staleness.** A `Current` call that starts later than the backend's bound after a `Bump` returned sees the bump.
- **Fail closed.** `Current` returns an error rather than a value older than the bound. Callers treat the error as "generation unknown" and read the
  source of truth instead of the cache.

## Backends

| Package | Shared | Staleness bound | Failure |
|---------|--------|-----------------|---------|
| [storages/memory](./storages/memory) | no | none | never fails |
| [storages/redis](./storages/redis) | yes | `maxStaleness` (1s) | see its README |

`data/cache` consumes a store through `cache.WithGeneration` (one namespace per `Cache`, `Cache.InvalidateAll`) and `cache.NewGenerations`
(several namespaces over one cache).
