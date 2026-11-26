# mirror

```go
import "github.com/altessa-s/go-atlas/auth/denylist/mirror"
```

Adapts an asynchronous, distributed revocation store to the **synchronous** [`jwt.RevocationChecker`](../../jwt) seam that `auth/jwt` and
`auth/selfjwt`
verifiers accept. It keeps a local snapshot of the revoked-key set, refreshed on a schedule, and answers every lookup from memory — so the verifier
hot
path never makes a network call and never blocks.

## Why

`jwt.RevocationChecker` is `IsRevoked(id string) (bool, error)` — synchronous, no context — because it runs on every token verification. The
distributed store ([`storages/redis.Store`](../storages/redis)) is `IsRevoked(ctx, key) (bool, error)` and cannot be called on that hot path. `mirror`
bridges the two: `Refresh` pulls the authoritative revoked set into a local snapshot; `IsRevoked` reads it lock-free.

## Types

| Type     | Description                                                                                                   |
|----------|---------------------------------------------------------------------------------------------------------------|
| `Cache`  | Synchronous, periodically-refreshed snapshot of a distributed denylist. Satisfies `jwt.RevocationChecker`.    |
| `Source` | Consumer-side seam the cache reads: `StreamValues(ctx) iter.Seq2[string, error]`. `redis.Store.RawSource()` supplies it. |

## API

| Symbol                                                | Description                                                                                                   |
|-------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `New(src, opt…) *Cache`                               | Cache backed by `src`. Starts unprimed and fails closed — call `Prime` before serving.                        |
| `Cache.Prime(ctx)`                                    | First synchronous refresh; returns the source error so a service can refuse to start.                          |
| `Cache.IsRevoked(id)`                                 | Lock-free check against the current snapshot; `jwt.ErrRevocationUnavailable` when unprimed or stale.          |
| `Cache.Refresh(ctx)`                                  | Rebuild the snapshot from `src` and swap it in atomically; on error the previous snapshot is kept.            |
| `Cache.Ready()` / `Cache.Age()`                       | Primed and fresh enough to answer; time since the last successful refresh (for health checks).                |
| `WithMaxStaleness(d)` / `WithClock(fn)`               | Oldest snapshot still trusted (default 5m; zero: no bound); time source.                                               |
| `WithInsecureServeUnprimed()`                         | Answer "not revoked" while unprimed instead of refusing.                                                      |
| `WithLogger(l)`                                       | Logs a change between serving and refusing, once per change.                                                  |
| `Cache.Len()`                                         | Keys in the current snapshot, for observability/tests.                                                        |
| `WithMetrics(m)` / `NewMetrics(collector, subsystem)` | Enable refresh telemetry (see [Metrics](#metrics)).                                                           |

## Staleness

A snapshot lags the authoritative store by up to one refresh interval: a key revoked on another node is not seen until the next `Refresh` — the same
propagation window any cached revocation set has. Size the refresh cadence to your revocation-propagation SLA. Because the store expires TTL-bounded
revocations natively, a refreshed snapshot only shrinks as entries expire; it never resurrects a dropped key. A failed `Refresh` keeps the previous
snapshot rather than emptying the denylist — a transient store outage degrades to staler data, never to "nothing is revoked". By
default (`DefaultMaxStaleness`, 5m; `WithMaxStaleness(0)` disables it) the snapshot stops being trusted once it is older than the bound, and verifiers reject tokens with
`jwt.ErrRevocationUnavailable` until a refresh succeeds. Before the first successful refresh the cache refuses to answer at all.

## Metrics

`WithMetrics(NewMetrics(collector, ""))` records, under subsystem `auth_denylist_mirror`:

| Metric            | Type    | Labels   | Meaning                                                                      |
|-------------------|---------|----------|------------------------------------------------------------------------------|
| `refreshes_total` | counter | `result` | Refresh outcomes: `ok` (new snapshot swapped in) / `error` (previous kept).  |
| `snapshot_size`   | gauge   | —        | Revoked-key count in the current snapshot after the last successful refresh. |
| `unavailable_total` | counter | `reason` | Lookups refused: `unprimed` (no refresh yet) or `stale` (older than the maximum staleness). |

A climbing `result="error"` rate flags a degraded source (the snapshot is going stale); watch `snapshot_size` for unexpected growth or a collapse to
zero. A nil collector or `*Metrics` makes every recording a zero-cost no-op, so metrics stay entirely optional.

## Usage

```go
store, err := redisstore.New(client)
if err != nil {
    return err
}
m := mirror.New(store.RawSource())
if err := m.Prime(ctx); err != nil {
    return err                       // prime before serving
}

v := jwt.NewVerifier(resolver, jwt.WithRevocation(m))

// Keep the snapshot fresh on a schedule (the go-horai task contract, a ticker, …):
_ = registrar.Register(ctx, task.Config{ID: "denylist-mirror", Schedule: "@every 30s", Func: m.Refresh})
```

Prefer [`factory/auth/denylist/mirror`](../../../factory/auth/denylist/mirror), which primes the cache, registers the refresh task and a health
checker.

## See also

- [`auth/denylist`](../) — the revocation seam and in-memory store.
- [`auth/denylist/storages/redis`](../storages/redis) — the authoritative distributed store this mirrors.
- [`auth/denylist/negcache`](../negcache) — the async read-path accelerator, when the verifier can afford an authoritative lookup on a filter hit;
  `mirror` instead keeps the whole set local for a fully synchronous check.
- [`auth/jwt`](../../jwt) — defines `RevocationChecker` and `WithRevocation`.
