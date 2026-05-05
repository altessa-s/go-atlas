# pool

```go
import "github.com/altessa-s/go-atlas/transport/grpc/client/pool"
```

Package `pool` provides gRPC client connection pooling with automatic cleanup and health monitoring. Each target address gets its own sub-pool of
connections bounded by the configured size. A background goroutine periodically evicts idle and unhealthy connections based on configurable
thresholds. Callers must return every connection obtained via `GetConnection` by calling `ReturnConnection`.

## Key types

| Type / Interface | Description                                                                                 |
|------------------|---------------------------------------------------------------------------------------------|
| `ConnectionPool` | Manages per-target connection pools with background cleanup; created via `New`               |
| `ClientFactory`  | Optional hook for custom connection creation; defaults to insecure dialer when not set       |

## Options

| Option                | Default | Description                                                                   |
|-----------------------|---------|-------------------------------------------------------------------------------|
| `WithSize`            | 10      | Maximum connections per target address                                        |
| `WithMaxIdleTime`     | 30m     | Maximum idle time before a connection is evicted                              |
| `WithCleanupInterval` | 5m      | Interval between background cleanup sweeps                                    |
| `WithConnectTimeout`  | 10s     | Timeout for establishing a new connection                                     |
| `WithLogger`          | discard | Structured logger for pool lifecycle events                                   |
| `WithClientFactory`   | nil     | Custom factory function for creating gRPC connections                         |
| `WithMetricsSubsystem` | `grpc_connection_pool` | Prometheus subsystem for emitted metrics; override per upstream so multiple pools can share one registry |
| `WithHealthCoordinator`     | nil          | Opts the pool into `observability/health` aggregate registration                                |
| `WithHealthServiceName`     | `grpc_pool`  | Service name registered with the coordinator                                                    |
| `WithPerTargetHealthChecks` | off          | Also register `<service>.<target>` checkers lazily on first conn for that target                |
| `WithHealthStateMapper`     | default      | Override the `connectivity.State` → `health.ServingStatus` mapping                              |

## Public API

| Method                          | Description                                                                       |
|---------------------------------|-----------------------------------------------------------------------------------|
| `Start(ctx)`                    | Starts background cleanup; returns a stop function that drains and closes        |
| `GetConnection(ctx, target)`    | Borrows or dials a conn for target                                                |
| `ReturnConnection(conn)`        | Returns a conn to the pool; unhealthy conns are closed                            |
| `SubscribeTarget(target, cb)`   | Push notifications for state changes; first call enables the state tracker      |
| `StateForTarget(target)`        | Synchronous best-of-conns connectivity state; returns `Idle` for unknown targets |
| `CheckHealth(ctx)`              | Implements `health.Checker` for the aggregate pool service                       |

## Health

When configured with `WithHealthCoordinator`, the pool aggregates the worst
per-target status into the registered service. The default
`connectivity.State` → `ServingStatus` mapping:

| `connectivity.State` | `ServingStatus` |
|----------------------|-----------------|
| `Ready` / `Idle` / `Connecting` | `Serving` |
| `TransientFailure`   | `Degraded`     |
| `Shutdown`           | `NotServing`   |

Coordinator subscribers receive an immediate push on every per-conn state
change. The watcher goroutine calls `NotifyStatusChange` synchronously —
gRPC does not hold any mutex while `WaitForStateChange` returns, so no
async indirection is required.

The state-tracking infrastructure is also exposed via `SubscribeTarget` and
`StateForTarget` so the gRPC client can observe pool state in pool mode
without configuring a coordinator on the pool itself.

## Errors

| Error                    | Description                                                              |
|--------------------------|--------------------------------------------------------------------------|
| `ErrConnectionPoolClosed`| Returned by `GetConnection` and `Start` after the pool has been stopped  |
