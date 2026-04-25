# Unique value store (`uniq`)

```go
import "github.com/altessa-s/go-atlas/data/uniq"
```

A small key-value store with one job: track that a key has been seen. Optionally hold an arbitrary serialized value alongside it. Three pluggable
backends: Redis, NATS JetStream KV, and a noop for tests. Default TTL is 24 hours; everything else is provider-specific.

---

## When to reach for `uniq`

| Scenario | Use |
|---|---|
| Webhook deduplication, idempotency keys, "did I already process this event ID" | `Add` + `Exist` |
| One-time tokens, magic links, password reset codes (with payload) | `AddWithValue` + `GetValue` |
| Free a slot early (token consumed, reset abused) | `Remove` |
| Wipe everything (test teardown, namespace reset) | `Clear` |

This is not a cache. There's no eviction policy beyond TTL, no LRU, no negative caching, no singleflight. If you need any of that, look at
`data/cache`. `uniq` answers a simpler question: "have I seen this key before?".

---

## Quick start

### Redis

```go
client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
defer client.Close()

u := uniq.NewWithRedis(client,
    uniq.WithLogger(logger),
    uniq.WithCollector(collector),
)

if err := u.Add(ctx, "event:42"); err != nil {
    return err
}

seen, err := u.Exist(ctx, "event:42")
```

### NATS JetStream

```go
nc, _ := nats.Connect(natsURL)
defer nc.Drain()

u, err := uniq.NewWithNats(nc,
    uniq.WithLogger(logger),
    uniq.WithCollector(collector),
)
if err != nil {
    return err
}
```

NATS provider auto-creates the bucket on first use. Bucket name and TTL are set via `nats.WithBucket` / `nats.WithTtl` on the provider itself,
not on `uniq`.

### No-op (tests)

```go
u := uniq.NewWithNoop()
require.NoError(t, u.Add(t.Context(), "any-key"))

exists, _ := u.Exist(t.Context(), "any-key")
require.False(t, exists) // noop never reports keys as existing
```

Use this when you want to wire `uniq` through a code path under test without standing up a real backend. Watch out: `Exist` always returns
`false`, so any test asserting "the second call short-circuits because the key is in the store" needs a real backend (miniredis covers Redis).

### Factory from YAML

```go
import uniqfactory "github.com/altessa-s/go-atlas/data/uniq/factory"

u, err := uniqfactory.New(cfg.CacheStorage).
    UseLogger(logger).
    UseRedisClient(redisClient).
    UseNatsConn(natsConn).
    UseCollector(collector).
    UseHealthCoordinator(healthCoord).
    Build()
```

YAML (`cache_storage.yaml` template):

```yaml
storage:
  type: redis    # or "nats", "memory"
  redis:
    keysPrefix: "myapp:uniq"
  nats:
    bucket: "myapp-uniq"
    replicas: 3
```

`type: memory` maps to the noop provider — fine for tests, useless in prod (no actual storage). The factory drops options on the floor unless
you hand it the matching client (`UseRedisClient` for `type: redis`, `UseNatsConn` for `type: nats`).

---

## Methods

| Method | Behavior |
|---|---|
| `Add(ctx, key)` | Insert key with the provider's default TTL. Overwrites if present |
| `AddWithValue(ctx, key, value)` | Same as Add, plus serialize `value` (default JSON) and store it |
| `Exist(ctx, key)` | Returns `true` if the key is present and not expired |
| `GetValue(ctx, key, out)` | Reads the value; returns `ErrDoesNotExist` for missing keys |
| `Remove(ctx, key)` | Deletes the key. No error if it didn't exist |
| `Clear(ctx)` | Wipes everything in the configured namespace. Use with care |

Keys are validated up front: empty rejected, longer than 1 KiB rejected with `ErrInvalidKey`. The 1 KiB cap is generous. If you're hitting it,
you're probably storing data inside the key.

`AddWithValue` / `GetValue` use the configured serializer (`WithSerializer`, defaults to JSON). For binary payloads, swap in a serializer that
skips JSON's base64 round-trip.

---

## TTL semantics

TTL is a property of the **provider**, not of `uniq` itself. Each provider has its own option:

- `redis.WithTtl(d)` — applied per `Add`/`AddWithValue` call as the value expiry.
- `nats.WithTtl(d)` — bucket-level TTL configured at bucket creation. Existing buckets keep their original TTL.
- `noop` — no TTL, nothing's stored.

There's no per-key TTL override at the `uniq` level. If you need a mix of long- and short-lived keys, run two `uniq` instances against different
namespaces.

---

## Health

`*Uniq` implements `health.Checker`. Pass `WithHealthCoordinator(c)` and `uniq` registers under `"uniq"` (or whatever `WithHealthServiceName`
overrides it to).

```go
coord := health.New()
u := uniq.NewWithRedis(client, uniq.WithHealthCoordinator(coord))
status := coord.CheckServiceHealth(ctx, "uniq") // health.StatusServing while Redis answers PING
```

`(*Uniq).CheckHealth` delegates to `providers.Prober`:

| Provider | What `Probe(ctx)` checks |
|---|---|
| `redis` | `client.Ping(ctx)` succeeds |
| `nats` | `kv.Status(ctx)` succeeds (bucket exists and is reachable) |
| `noop` | Always healthy. Nothing else to check |

A third-party provider that doesn't implement `providers.Prober` is reported as healthy. The type assertion in `CheckHealth` falls into the
`Serving` branch. Failing readiness on an unknown implementation would block deploys for no diagnosable reason, so we err the other way and
let you wire your own probe if you need one.

Two `uniq` instances in one process with separate health entries:

```go
uTokens := uniq.NewWithRedis(client,
    uniq.WithHealthCoordinator(coord),
    uniq.WithHealthServiceName("uniq-tokens"),
)
uEmails := uniq.NewWithNats(nc,
    uniq.WithHealthCoordinator(coord),
    uniq.WithHealthServiceName("uniq-emails"),
)
```

---

## Metrics

Subsystem `uniq`. Every public method is instrumented with the `op` label.

| Metric | Type | Labels |
|---|---|---|
| `uniq_operations_total` | counter | `op` |
| `uniq_operation_duration_seconds` | histogram | `op` |
| `uniq_operation_errors_total` | counter | `op` |

`op` values: `add`, `add_with_value`, `exist`, `get_value`, `remove`, `clear`.

Failure-rate dashboards: `uniq_operation_errors_total / uniq_operations_total` per `op`. Spikes on `exist` usually mean Redis or NATS is
dropping connections. Spikes on `add_with_value` more often point at serialization failures; check the error chain for
`ErrSerializationFailed`, which wraps the underlying serializer error.

---

## Failure modes

**`ErrInvalidKey`.** Key is empty or longer than 1 KiB. Validate inputs upstream; don't shove user-controlled blobs into the key.

**`ErrDoesNotExist` from `GetValue`.** Key isn't there or has expired. Treat as "first time we see this".

**`ErrSerializationFailed` / `ErrDeserializationFailed`.** The serializer rejected the payload. JSON struggles with `chan`, `func`, recursive
types, `time.Time` with custom layouts, and big integers. Use `WithSerializer` to plug in something else (msgpack, gob, protobuf).

**Health flip-flop on Redis.** Every probe runs `PING`. If Redis is overloaded enough to drop `PING`, your application calls are also
suffering. Fix Redis; don't suppress the probe.

**`uniq.Exist` always returns false.** You're using the noop provider. Either you wired the wrong factory option (`type: memory` produces a
noop) or you're in a test that doesn't have a real backend.

---

## Related docs

- [`docs/configuration.md`](configuration.md) — overall YAML format and `cache_storage.yaml` template
- [`docs/health.md`](health.md) — how `health.Coordinator` polls registered services
- [`docs/metrics.md`](metrics.md) — full metric reference for the repository
- [`data/uniq/README.md`](../data/uniq/README.md) — godoc-level option reference
