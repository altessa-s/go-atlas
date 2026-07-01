# factory

```go
import "github.com/altessa-s/go-atlas/auth/denylist/negcache/factory"
```

Package `factory` provides a fluent builder that constructs a [`negcache.Cache`](../) from a probabilistic-filter configuration plus injected
dependencies. It builds the negative filter through the shared [probfilter factory](../../../../data/probfilter/factory) and wires it to a
caller-supplied `negcache.Authoritative` revocation store. Errors from any step are collected and returned at `Build()` time.

The factory never builds the authoritative store or a Redis client — following the inject-deps-at-the-consumer convention, the caller injects
those (for example an `auth/denylist/storages/redis.Store`, which satisfies `negcache.Authoritative`) and the factory only assembles the cache
around them.

## Quick Start

```go
cache, err := factory.NewBuilder("denylist", cfg.Filter, &defaults, redisDenylist).
    UseLogger(logger).
    UseRedisClient(redisClient). // only for a Redis-backed negative filter
    UseMetrics(collector, "auth_denylist_negcache").
    Build()
```

## Constructor

| Method | Description |
|--------|-------------|
| `NewBuilder(name, filterCfg, defaults, authoritative)` | Creates a `Builder` for a cache named `name`, built from `filterCfg`/`defaults` and fronting `authoritative`. The name scopes the probfilter's metrics and storage keys. |

## Dependencies

| Method | Required | Description |
|--------|----------|-------------|
| `authoritative` (constructor arg) | yes | The exact revocation store the cache fronts. Injected, never built by the factory. |
| `filterCfg` (constructor arg) | yes | The probabilistic-filter configuration used to build the negative filter. |
| `UseLogger` | no | Sets the logger for the builder and the filter it constructs. |
| `UseRedisClient` | only for Redis-backed filters | Redis client passed through to the probfilter factory. In-memory filters need none. |
| `UseMetrics` | no | Enables lookup telemetry on the built cache using the given collector and subsystem. |

## Terminal

| Method | Description |
|--------|-------------|
| `Build` | Validates the injected dependencies, builds the negative filter, and returns the assembled `*negcache.Cache`. |

## See also

- [`negcache`](../) — the negative cache this factory assembles.
- [`data/probfilter/factory`](../../../../data/probfilter/factory) — the filter builder driven here.
