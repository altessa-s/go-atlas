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

## Errors

| Error                    | Description                                                              |
|--------------------------|--------------------------------------------------------------------------|
| `ErrConnectionPoolClosed`| Returned by `GetConnection` and `Start` after the pool has been stopped  |
