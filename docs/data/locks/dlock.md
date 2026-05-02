# Distributed locks (`dlock`)

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock"
```

A distributed mutex for coordinating work across instances of the same service. NATS JetStream KV under the hood, with an optional noop for tests.
One owner per key, automatic lease renewal while you hold it, and a fencing token if you lose leadership without noticing.

---

## When to reach for `dlock`

| Scenario | Use |
|---|---|
| "Only one instance should do X right now" | `dl.Synchronize(ctx, key, fn)` — acquires, runs, releases |
| Long critical section spanning several functions | `dl.Lock(ctx, key)` + `defer lock.Release(ctx)` |
| Need to inspect who holds the lock | `dl.GetLockInfo(ctx, key)` → owner, acquired_at, fencing token |
| Test without network infrastructure | `dlock.NewWithNoop()` — operations succeed without external calls |

This is not consensus. If you need a single owner with linearisable writes, look at raft or etcd. `dlock` gives you mutual exclusion with a TTL.
Once that TTL expires without a renew, another instance takes the key, even if you're still alive.

---

## Quick start

### NATS JetStream

```go
nc, _ := nats.Connect(natsURL)
defer nc.Drain()

dl, err := dlock.NewWithNats(ctx, nc, "myapp-locks",
    dlock.WithLogger(logger),
    dlock.WithCollector(collector),
)
if err != nil {
    return err
}
defer dl.Close(ctx)

err = dl.Synchronize(ctx, "rebuild-cache", func(ctx context.Context) error {
    return rebuildCache(ctx)
})
```

`Synchronize` handles the full lifecycle: acquire → run → release, with double-release protection and panic recovery inside `fn`. If `fn` panics,
the lock is still released and the panic is logged via `WithLogger`.

### No-op (tests)

```go
dl := dlock.NewWithNoop()
require.NoError(t, dl.Synchronize(ctx, "any-key", func(ctx context.Context) error {
    return doWork(ctx)
}))
```

Every operation succeeds without a network call. `GetLockInfo` returns synthetic metadata with owner = `"nop"`. Use it for unit tests of layers
where the call to `dlock` matters more than real coordination.

### Factory from YAML

```go
import dlockfactory "github.com/altessa-s/go-atlas/data/locks/dlock/factory"

dl, err := dlockfactory.New(cfg.DistributionLock).
    UseLogger(logger).
    UseNatsConn(natsConn).
    UseHealthCoordinator(healthCoord).
    Build(ctx)
```

YAML:

```yaml
distributionLock:
  provider: nats
  nats:
    bucket: myapp-locks
```

The factory picks a provider from `cfg.Provider`. Only `nats` is wired today; new providers go through extending the `DistributionLockProvider`
enum in `config/dlock.go` and adding a branch to `factory/builder.go::Build`.

---

## Synchronize vs Lock

| Method | When |
|---|---|
| `Synchronize(ctx, key, fn)` | The critical section fits in one function. `dlock` owns the lifecycle |
| `Lock(ctx, key)` → `(providers.Lock, error)` | The section spans several functions or modules |

`Lock` returns a `providers.Lock` with only `Release(ctx)` and `GetLockInfo(ctx)`. The `defer lock.Release(ctx)` discipline is on you. If you
forget, the lock eventually releases itself when the TTL expires (10 seconds by default for the NATS bucket), but every other instance is
waiting that whole time.

---

## TTL, renew, fencing

The NATS provider stores the lease in a KV bucket with a TTL and renews it in the background every `TTL × RenewRatio` seconds (default
`0.75 × 10s = 7.5s`). If the process dies or loses connectivity, the next renew fails, the key expires after the TTL, and another instance
takes it.

`GetLockInfo` exposes a `FencingToken`, a monotonically increasing revision from NATS KV. If you do an out-of-band side effect tied to lock
ownership (writing to another database, publishing to a queue), have the receiver check that the fencing token is at least as large as the last
one it saw. Without that check, an old owner reconnecting after a pause longer than the TTL can flood the system with stale operations.

`WithLockAcquireTimeout(d)` caps the time spent **acquiring** the lock, not how long you hold it. Default is 30 seconds. On expiry you get a
`coreerrs.IsContextDeadlineExceeded`-compatible error wrapped as `failed to acquire lock within timeout 30s`.

---

## Health

`*DLock` implements `health.Checker`. Pass `WithHealthCoordinator(c)` and `dlock` registers itself under the name `"dlock"` (or whatever
`WithHealthServiceName` overrides it to).

```go
coord := health.New()
dl := dlock.NewWithNoop(dlock.WithHealthCoordinator(coord))
status := coord.CheckServiceHealth(ctx, "dlock") // health.StatusServing
```

Internally `(*DLock).CheckHealth` delegates to `providers.Prober`:

| Provider | What `Probe(ctx)` checks |
|---|---|
| `nats` | Not closed; `client.Status() == CONNECTED`; `kv.Status(ctx)` responds (bucket exists and is reachable) |
| `noop` | Not closed. Nothing else to probe — it's a stub |

A third-party provider that doesn't implement `providers.Prober` is reported as healthy by default. The type assertion in `CheckHealth` falls
into the `Serving` branch — failing readiness on an unknown implementation would block deploys for no diagnosable reason, so we err the other
way and let you wire your own probe if you need one.

Multiple `dlock` instances in one process with separate health reporting:

```go
dlPayments := dlock.NewWithNats(ctx, nc, "payments-locks",
    dlock.WithHealthCoordinator(coord),
    dlock.WithHealthServiceName("dlock-payments"),
)
dlInventory := dlock.NewWithNats(ctx, nc, "inventory-locks",
    dlock.WithHealthCoordinator(coord),
    dlock.WithHealthServiceName("dlock-inventory"),
)
```

---

## Metrics

Subsystem `dlock` (see also [`docs/metrics.md`](../../metrics.md)):

| Metric | Type | When it increments |
|---|---|---|
| `dlock_locks_acquired_total` | counter | Lock acquired |
| `dlock_locks_released_total` | counter | Release succeeded |
| `dlock_locks_failed_total` | counter | Lock acquisition returned an error |
| `dlock_acquire_duration_seconds` | histogram | Time from `Lock` call to return (success or error) |
| `dlock_synchronizations_total` | counter | `Synchronize` finished (including cases where `fn` returned an error) |

Plot `acquired - released - in_flight` in a dashboard to surface leaks: locks that were taken but never released, usually from a panic outside
`Synchronize` or a forgotten `defer Release`. Don't read a small surplus of `released` over `acquired` as a leak — that happens normally when one
node takes the lock, crashes, the TTL releases the key, and another node releases its own lock cleanly later on.

---

## Failure modes

**`failed to acquire lock within timeout 30s`.** Someone holds the lock longer than `TTL × RenewRatio`, or NATS connectivity is dropping. Raise
`WithLockAcquireTimeout` or break the work into smaller chunks.

**`errs.ErrLockNotHeld` from `GetLockInfo`.** The key doesn't exist or expired. Treat it as "the lock is free"; it's not an error.

**`provider is closed`.** Someone called `dl.Close()` too early. Fix the lifecycle of whatever owns the `DLock`.

**Health flip-flop (Serving ↔ NotServing).** Flaky NATS connection, or a KV bucket that goes intermittently unavailable. Investigate the NATS
cluster; don't gate readiness on `dlock` alone.

**Double execution of `fn` inside `Synchronize`.** The TTL expired while `fn` was still running, and another node grabbed the lock. Shrink the
work done under the lock, or raise `nats.WithTTL`.

---

## Related docs

- [`docs/configuration.md`](../../configuration.md) — overall YAML format and where `dlock.yaml` fits
- [`docs/observability/health.md`](../../observability/health.md) — how `health.Coordinator` polls registered services
- [`docs/metrics.md`](../../metrics.md) — the full metric reference for the repository
