# factory

```go
import "github.com/altessa-s/go-atlas/factory/auth/denylist/mirror"
```

Package `factory` provides a fluent builder that constructs a [`mirror.Cache`](../../../../auth/denylist/mirror/) around a revocation source and wires its
lifecycle: priming, the refresh task and a health checker.

## Quick Start

```go
cache, err := factory.NewBuilder(store.RawSource()).
    UseScheduler(scheduler).             // required: runs the refresh task
    UseHealthCoordinator(coordinator).   // NotServing while unprimed or stale
    UseRefreshSchedule("@every 15s").    // the revocation-propagation window
    UseMaxStaleness(time.Minute).        // refuse to answer from an older snapshot
    Build(ctx)
if err != nil {
    return err // the revocation source was unreachable: do not serve
}
defer cache.Close() // stops the refresh task and releases the health registration
verifier := jwt.NewVerifier(resolver, jwt.WithRevocation(cache))
```

## Behavior

| Step                  | Effect                                                                                                                         |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `Build` priming       | One synchronous refresh with the build context, bounded by the prime timeout. A source error fails `Build` (matchable with `errors.Is`).                    |
| Refresh task          | Task `denylist-mirror-refresh-<name>` (default `@every 30s`, 20 s timeout) registered with the scheduler; `Build` fails without one. Each mirror needs its own name (`UseName`; unnamed mirrors are numbered) because a scheduler replaces a task registered under an existing ID. |
| Health checker        | Service `denylist.mirror.<name>`: Serving while `cache.Ready()`, NotServing while unprimed or older than the maximum staleness.      |
| `Close`               | `Build` returns a `*Managed` (the `mirror.Cache` plus its registrations). `Close` makes the refresh task a no-op (it stays registered but inert, because a named mirror shares its task ID across replicas) and releases the scoped health registration. Safe to repeat. |
| Unavailable           | `cache.IsRevoked` returns `jwt.ErrRevocationUnavailable`; verifiers reject the token, distinct from `jwt.ErrTokenRevoked`.    |
| `UseInsecureServeUnprimed` | Opt-out: `Build` survives a priming failure and the cache answers "not revoked" until a refresh succeeds.            |

## Builder methods

| Method                       | Description                                                     |
|------------------------------|-----------------------------------------------------------------|
| `NewBuilder(src)`            | Builder for a mirror reading `src` (a `mirror.Source`).         |
| `UseScheduler(r)`            | Scheduler that runs the refresh task. Required.                 |
| `UseHealthCoordinator(r)`    | Registrar for the health checker.                               |
| `UseHealthServiceName(name)` | Overrides the service name `denylist.mirror`.                   |
| `UseRefreshSchedule(spec)`   | Cron expression of the refresh task.                            |
| `UseMaxStaleness(d)`         | Oldest snapshot still trusted; defaults to 5m, zero disables the bound. `Build` rejects a value below an `@every` refresh interval. |
| `UseName(name)`              | Suffix of the task ID and health service name; distinguishes several mirrors on one scheduler. |
| `UsePrimeTimeout(d)`         | Bound of the priming refresh (default 20 s).                    |
| `UseInsecureServeUnprimed()` | Serve "not revoked" while unprimed.                             |
| `UseMetrics(meter, subsys)`  | Refresh and refusal telemetry.                                  |
| `UseLogger(l)`               | Logger of the builder and the cache.                            |
