# dlock

```go
import "github.com/altessa-s/go-atlas/coord/dlock"
```

Package `dlock` provides distributed locks for coordinating access across multiple service instances. Supports pluggable providers (NATS JetStream,
MongoDB, SQL, no-op) with automatic resource management and context-aware operations.

## Options

| Option                  | Default   | Description                                                      |
|-------------------------|-----------|------------------------------------------------------------------|
| `WithLogger`            | discard   | Structured logger                                                |
| `WithMeter`             | noop      | Prometheus metrics collector                                     |
| `WithHealthCoordinator` | unset     | Auto-register with a `health.Registrar` on construction          |
| `WithHealthServiceName` | `"dlock"` | Override service name when several DLock instances share a coord |
| `WithReleaseTimeout`    | `10s`     | Bound of the release `Synchronize` performs; non-positive values are ignored and keep the default |

## Acquisition and lifetime

`Lock` makes a **single attempt**. When the key is held it returns `providers.ErrLockNotHeld` straight away rather than waiting for the holder, so
callers
that need to be serialized rather than rejected must retry themselves.

The context passed to `Lock` (and to `Synchronize`) **scopes the lock**: cancel it and the lease stops being renewed and is released. Never hand a
lock
a context that ends before the work it guards. The acquisition attempt is bounded separately, by the NATS provider's `WithAcquireTimeout`, because a
bound folded into the same context would release the lock the moment it elapsed.

A failed renewal is retried for as long as the lease can still be valid; the lock is lost only when that window closes or the key is gone or owned by
someone else. See [docs/coord/dlock.md](../../docs/coord/dlock.md#ttl-renew-fencing).

## Lost locks in `Synchronize`

`Synchronize` runs its function under a context derived from the caller's. When the lease is lost while the function runs — it expired, was taken
over or could not be renewed in time — that context is canceled with cause `dlock.ErrLockLost` (`context.Cause(ctx)`), and `Synchronize` returns an
error that matches `dlock.ErrLockLost` joined with the function's own error. The function must stop when its context ends: the key may already have
another holder. The signal comes from locks that implement `providers.LostNotifier`; the `mongo` and `sqldb` providers do, while under `nats`, `noop`
and third-party providers without it the function runs to the end as before, so guard its writes with the fencing token.

The release uses its own timeout from `WithReleaseTimeout` and runs even when the function panics. A panic returns `dlock.ErrPanicked`. Release
failures are logged and joined with the function error, panic, or lease loss.

## Metrics

Subsystem `dlock`:

| Metric                     | Type      | Description                                     |
|----------------------------|-----------|-------------------------------------------------|
| `locks_acquired_total`     | counter   | Locks successfully acquired                     |
| `locks_released_total`     | counter   | Locks successfully released (paired counter)    |
| `locks_failed_total`       | counter   | Lock-acquisition failures                       |
| `acquire_duration_seconds` | histogram | Provider acquisition durations in `Synchronize` |
| `synchronizations_total`   | counter   | `Synchronize` calls completed                   |

All of these are recorded by `Synchronize` only; a direct `Lock` call delegates to the provider without instrumentation.

`acquired - released - in_flight` surfaces leaked locks in dashboards.

## Health

`*DLock` implements `health.Checker`. When constructed with `WithHealthCoordinator`, it auto-registers under the configured service name (default
`"dlock"`). Health probes delegate to `providers.Prober` when the provider implements it; in-tree providers (`nats`, `mongo`, `sqldb`, `noop`) do. The
NATS prober checks both connection state and KV bucket reachability, so a missing or wedged bucket flips readiness independently of the TCP
connection.

## Subpackages

| Package                              | Description                         |
|--------------------------------------|-------------------------------------|
| [factory](../../factory/coord/dlock) | Configuration-based creation        |
| [providers/mongo](./providers/mongo) | MongoDB provider                    |
| [providers/nats](./providers/nats)   | NATS JetStream provider             |
| [providers/noop](./providers/noop)   | No-op provider for testing          |
| [providers/sqldb](./providers/sqldb) | PostgreSQL, MySQL, MariaDB provider |
