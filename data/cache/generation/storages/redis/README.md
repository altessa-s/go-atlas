# redis

```go
import "github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
```

Package `redis` provides a `generation.Store` shared through Redis: a bump on one replica invalidates the generation-scoped caches of every
replica that uses the same Redis and key prefix.

```go
store := redis.New(client, redis.WithMaxStaleness(500*time.Millisecond))
c := cache.New(storage, cache.WithGeneration(store, "catalog"))
```

## Semantics

| Operation | Redis | Local |
|-----------|-------|-------|
| `Bump` | Lua: `SET max(stored+1, seen+1, server µs)`, `seen` = highest generation this replica observed | visible to this replica at once |
| `Current` | Lua: `GET`, seeding a missing key with the server time in µs | reused for at most `maxStaleness`; concurrent refreshes collapse |

**Staleness bound.** A `Current` call that starts more than `maxStaleness` after a `Bump` returned, on any replica, returns at least the bumped
value. The check time of a local value is the *start* of the Redis call that produced it, so the bound does not grow with Redis latency. A collapsed
refresh that started before a caller's window is not reused by that caller.

**Monotonic.** A missing key (flushed or failed-over Redis) is seeded from the Redis server clock (`TIME` inside the script, Redis 5+), never from
zero and never from a replica clock, so replica clock skew plays no role. A bump always exceeds every generation the bumping replica has seen, even
when Redis holds less.

**Regression.** When a read that started after this replica confirmed generation `G` returns less than `G` (a failover restored an older value),
`Current` fails closed with `ErrRegressed`, keeps its local value without refreshing its check time, and heals the key with a bump above `G`; later
reads resume from the healed value.

**Overlap rule.** Per namespace the store keeps a count of finished bumps and of bumps in flight. A read overlapped by a bump of this replica
(a bump finished since it started, or one was in flight when it started or completed) never installs its value: a value at or below the current
generation is discarded, a higher one fails closed with `ErrRegressed` and heals as above (it may predate the write; a concurrent bump of
another replica also lands here, at the cost of one extra bump). Every read raises the observed maximum. `Bump` succeeds only with a value it
published that exceeds every generation the replica observed by the time it completes (it republishes, at most 3 times, otherwise fails and
stays pending).

Residual window: a replica that never observed `G` cannot detect the regression and may serve entries cached
under the restored generation until a healing bump reaches it (at most `maxStaleness` after a replica that knew `G` refreshes) or until those
entries expire. A finite entry TTL is therefore required.

**Why a local TTL and not pub/sub.** The bound holds without a subscriber connection, reconnect handling or delivery guarantees, and the cost is at
most one `GET` per namespace per `maxStaleness`.

## Failures

| Situation | Behavior |
|-----------|----------|
| Redis unreachable, local value younger than `maxStaleness` | the local value (within the bound) |
| Redis unreachable, local value older | `Current` returns an error; the cache is skipped |
| `Bump` fails | the error; the namespace is pending on this replica (see below) |

A pending namespace makes `Current` retry the increment and return an error until it reaches Redis, so this replica never serves what the write
replaced. Other replicas cannot learn of an unpublished bump: for them the bound degrades to the lifetime (TTL) of their cached entries. Keep the
entry TTL as the safety net.

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithKeyPrefix` | `generation:` | Prefix of the generation keys |
| `WithMaxStaleness` | 1s | Local reuse bound; zero reads Redis on every `Current`, negative values are ignored |
| `WithClock` | `clock.Real()` | Clock for staleness (seeding uses the Redis server clock) |

Generation keys have no TTL: they must outlive every entry cached under them. There is one key per namespace.
