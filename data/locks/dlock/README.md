# dlock

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock"
```

Package `dlock` provides distributed locks for coordinating access across multiple service instances. Supports pluggable providers (NATS
JetStream, no-op) with automatic resource management and context-aware operations.

## Options

| Option                   | Default   | Description                                                      |
|--------------------------|-----------|------------------------------------------------------------------|
| `WithLockAcquireTimeout` | 30s       | Timeout for lock acquisition                                     |
| `WithLogger`             | discard   | Structured logger                                                |
| `WithCollector`          | noop      | Prometheus metrics collector                                     |
| `WithHealthCoordinator`  | unset     | Auto-register with `health.Coordinator` on construction          |
| `WithHealthServiceName`  | `"dlock"` | Override service name when several DLock instances share a coord |

## Metrics

Subsystem `dlock`:

| Metric                       | Type      | Description                                  |
|------------------------------|-----------|----------------------------------------------|
| `locks_acquired_total`       | counter   | Locks successfully acquired                  |
| `locks_released_total`       | counter   | Locks successfully released (paired counter) |
| `locks_failed_total`         | counter   | Lock-acquisition failures                    |
| `acquire_duration_seconds`   | histogram | Distribution of `Lock` durations             |
| `synchronizations_total`     | counter   | `Synchronize` calls completed                |

`acquired - released - in_flight` surfaces leaked locks in dashboards.

## Health

`*DLock` implements `health.Checker`. When constructed with
`WithHealthCoordinator`, it auto-registers under the configured service
name (default `"dlock"`). Health probes delegate to `providers.Prober`
when the provider implements it; in-tree providers (`nats`, `noop`) do.
The NATS prober checks both connection state and KV bucket reachability,
so a missing or wedged bucket flips readiness independently of the TCP
connection.

## Subpackages

| Package                              | Description                          |
|--------------------------------------|--------------------------------------|
| [factory](./factory)                 | Configuration-based creation         |
| [providers/nats](./providers/nats)   | NATS JetStream provider              |
| [providers/noop](./providers/noop)   | No-op provider for testing           |
| [errs](./errs)                       | Error definitions                    |
