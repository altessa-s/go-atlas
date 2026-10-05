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

`NewWithNats` sets only the bucket. To pass other provider options (`WithTTL`, `WithMigrateBucketTTL`, …), build the provider from
`data/locks/dlock/providers/nats` (imported as `natsprovider` here) and wrap it, as `dlock/factory` does:

```go
prov, err := natsprovider.New(ctx, nc,
    natsprovider.WithBucket("myapp-locks"),
    natsprovider.WithTTL(30*time.Second),
)
if err != nil {
    return err
}
dl := dlock.New(prov, dlock.WithLogger(logger))
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
    storage: memory           # or file: keep the bucket and the fencing-token sequence across a server restart
    migrateBucketTTL: false   # true updates an existing bucket whose key TTL differs (see "TTL, renew, fencing")
```

The factory picks a provider from `cfg.Provider`. Only `nats` is wired today; new providers go through extending the `DistributionLockProvider`
enum in `config/dlock.go` and adding a branch to `factory/builder.go::Build`.

---

## Synchronize vs Lock

| Method | When |
|---|---|
| `Synchronize(ctx, key, fn)` | The critical section fits in one function. `dlock` owns the lifecycle |
| `Lock(ctx, key)` → `(providers.Lock, error)` | The section spans several functions or modules |

`Lock` returns a `providers.Lock` with only `Release(ctx)` and `GetLockInfo(ctx)`. The `defer lock.Release(ctx)` discipline is on you. The NATS
provider keeps renewing the lease in the background for as long as the lock's context lives and the connection is healthy, so a forgotten
`Release` under `context.Background()` can hold the lock indefinitely. The TTL (10 seconds by default for the NATS bucket) frees it only after
renewal stops — the process dies, the connection drops, or the context passed to `Lock` is canceled. Always `Release`, or bound the lock's
lifetime with a cancelable context.

---

## TTL, renew, fencing

The NATS provider stores the lease in a KV bucket whose key TTL is the configured lock TTL, and renews it in the background every
`TTL × RenewRatio` seconds (default `⅓ × 10s ≈ 3.3s`). If the process dies or loses connectivity, the next renew fails, the key expires after the
TTL, and another instance takes it.

A failed renew is retried with exponential backoff for as long as the lease can still be valid: until the start of the last successful write plus
the TTL, less a safety margin of 10% of the TTL that absorbs clock drift and scheduling delay. Each attempt is bounded by that deadline, so a
request that is never answered cannot keep the holder believing it owns an expired lock. The lock is reported lost — and stops renewing — when
the window closes, or at once when the failure is definitive: the key is gone, another owner holds it, its revision moved on, or the NATS connection
was closed. The ratio therefore decides how much of the TTL is left for retries: at the default `⅓` a renewal that starts failing still has about
two thirds of the TTL to recover. Ratios above `0.8` are clamped to `0.8` so a renewal always starts before the deadline.

The window is computed from the provider's configured TTL, which is also the bucket's key TTL, so every provider sharing a bucket must use the same
`WithTTL`. `New` enforces it: when the bucket already exists with a different key TTL it returns `nats.ErrBucketTTLMismatch` and leaves the bucket
alone, rather than rewriting it and silently shortening or stretching the locks of the providers already using it. Pass `WithMigrateBucketTTL()` to
change the TTL of an existing bucket deliberately — once every provider sharing it is configured with the new TTL. A lock built from YAML through
`dlock/factory` takes the same switch as `distributionLock.nats.migrateBucketTTL: true`.

The bucket is memory-backed unless the provider is built with `WithStorage(jetstream.FileStorage)`. The server cannot change an existing bucket's
storage type, so the provider's `MigrateBucketStorage` recreates the bucket instead, while every user is stopped. Its first revision is set just
above the old bucket's last one, so fencing tokens keep growing across the move. The provider README has the procedure.

`GetLockInfo` exposes a `FencingToken`, a monotonically increasing revision from NATS KV. If you do an out-of-band side effect tied to lock
ownership (writing to another database, publishing to a queue), have the receiver check that the fencing token is at least as large as the last
one it saw. Without that check, an old owner reconnecting after a pause longer than the TTL can flood the system with stale operations.

The provider's `WithAcquireTimeout(d)` caps the **acquisition attempt** — the round trips it takes to try the key once — and nothing else. Default
is 5 seconds. It deliberately lives on the provider rather than on `DLock`: the only lever `DLock` has is the context it passes down, and that
context scopes the lock's lifetime, so an acquisition deadline folded into it would release the lock the moment it elapsed, mid critical section.

That is also why the context you hand to `Lock` or `Synchronize` must outlive the work: canceling it releases the lock.

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
dlPayments, err := dlock.NewWithNats(ctx, nc, "payments-locks",
    dlock.WithHealthCoordinator(coord),
    dlock.WithHealthServiceName("dlock-payments"),
)
if err != nil {
    return err
}
dlInventory, err := dlock.NewWithNats(ctx, nc, "inventory-locks",
    dlock.WithHealthCoordinator(coord),
    dlock.WithHealthServiceName("dlock-inventory"),
)
if err != nil {
    return err
}
```

---

## Metrics

Subsystem `dlock` (see also [`docs/metrics.md`](../../metrics.md)):

| Metric | Type | When it increments |
|---|---|---|
| `dlock_locks_acquired_total` | counter | `Synchronize` acquired the lock |
| `dlock_locks_released_total` | counter | `Synchronize` released the lock successfully |
| `dlock_locks_failed_total` | counter | `Synchronize` lock acquisition returned an error |
| `dlock_acquire_duration_seconds` | histogram | Time for `Synchronize` to acquire the lock (success or error) |
| `dlock_synchronizations_total` | counter | `Synchronize` finished (including cases where `fn` returned an error) |

All metrics are recorded inside `Synchronize` only. `Lock` and the `providers.Lock` it returns record nothing, so locks taken through `Lock` —
including one whose `defer Release` was forgotten — are invisible here; instrument that path at your call site if you need it.

`acquired - released` counts outstanding `Synchronize` acquisitions: critical sections still running plus locks whose release failed. A
long-running `fn` keeps the difference positive while healthy, and counters reset on process restart, so compare increases over the same window
(`increase(acquired[5m]) - increase(released[5m])`) and add your own in-flight gauge before treating a persistent gap as a leak.

---

## Failure modes

**`errs.ErrLockNotHeld` from `Lock`.** Someone else holds the key. Acquisition does not wait, so this is the normal answer under contention — retry
with backoff if you need a turn rather than a rejection.

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
