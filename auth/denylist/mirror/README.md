# mirror

```go
import "github.com/altessa-s/go-atlas/auth/denylist/mirror"
```

Adapts an asynchronous, distributed revocation store to the **synchronous** [`jwt.RevocationChecker`](../../jwt) seam that `auth/jwt` and
`auth/selfjwt` verifiers accept. It keeps a local snapshot of the revoked-key set, refreshed on a schedule, and answers every lookup from
memory — so the verifier hot path never makes a network call and never blocks.

## Why

`jwt.RevocationChecker` is `IsRevoked(id string) bool` — synchronous, no context, no error — because it runs on every token verification.
The distributed store ([`storages/redis.Store`](../storages/redis)) is `IsRevoked(ctx, key) (bool, error)` and cannot be called on that hot
path. `mirror` bridges the two: `Refresh` pulls the authoritative revoked set into a local snapshot; `IsRevoked` reads it lock-free.

## Types

| Type     | Description                                                                                                  |
|----------|--------------------------------------------------------------------------------------------------------------|
| `Cache`  | Synchronous, periodically-refreshed snapshot of a distributed denylist. Satisfies `jwt.RevocationChecker`.    |
| `Source` | Consumer-side seam the cache reads: `StreamValues(ctx) iter.Seq2[string, error]`. `redis.Store` satisfies it. |

## API

| Symbol                    | Description                                                                                       |
|---------------------------|---------------------------------------------------------------------------------------------------|
| `New(src, opt…) *Cache`   | Cache backed by `src`. Snapshot starts empty — call `Refresh` before serving.                     |
| `Cache.IsRevoked(id)`     | Lock-free membership check against the current snapshot (satisfies `jwt.RevocationChecker`).       |
| `Cache.Refresh(ctx)`      | Rebuild the snapshot from `src` and swap it in atomically; on error the previous snapshot is kept. |
| `Cache.Len()`             | Keys in the current snapshot, for observability/tests.                                             |
| `WithMetrics(m)` / `NewMetrics(collector, subsystem)` | Enable refresh telemetry (see [Metrics](#metrics)).                    |

## Staleness

A snapshot lags the authoritative store by up to one refresh interval: a key revoked on another node is not seen until the next `Refresh` —
the same propagation window any cached revocation set has. Size the refresh cadence to your revocation-propagation SLA. Because the store
expires TTL-bounded revocations natively, a refreshed snapshot only shrinks as entries expire; it never resurrects a dropped key. A failed
`Refresh` keeps the previous snapshot rather than emptying the denylist — a transient store outage degrades to staler data, never to
"nothing is revoked".

## Metrics

`WithMetrics(NewMetrics(collector, ""))` records, under subsystem `auth_denylist_mirror`:

| Metric            | Type    | Labels   | Meaning                                                                       |
|-------------------|---------|----------|-------------------------------------------------------------------------------|
| `refreshes_total` | counter | `result` | Refresh outcomes: `ok` (new snapshot swapped in) / `error` (previous kept).   |
| `snapshot_size`   | gauge   | —        | Revoked-key count in the current snapshot after the last successful refresh.  |

A climbing `result="error"` rate flags a degraded source (the snapshot is going stale); watch `snapshot_size` for unexpected growth or a
collapse to zero. A nil collector or `*Metrics` makes every recording a zero-cost no-op, so metrics stay entirely optional.

## Usage

```go
store := redisstore.New(client)     // authoritative distributed store (mirror.Source)
m := mirror.New(store)
if err := m.Refresh(ctx); err != nil {
    return err                       // prime before serving
}

v := jwt.NewVerifier(resolver, jwt.WithRevocation(m))

// Keep the snapshot fresh on a schedule (core/scheduler, a ticker, …):
_ = registrar.Register("denylist-mirror", m.Refresh)
```

## See also

- [`auth/denylist`](../) — the revocation seam and in-memory store.
- [`auth/denylist/storages/redis`](../storages/redis) — the authoritative distributed store this mirrors.
- [`auth/denylist/negcache`](../negcache) — the async read-path accelerator, when the verifier can afford an authoritative lookup on a
  filter hit; `mirror` instead keeps the whole set local for a fully synchronous check.
- [`auth/jwt`](../../jwt) — defines `RevocationChecker` and `WithRevocation`.
