# health

```go
import "github.com/altessa-s/go-atlas/transport/grpc/handlers/health"
```

Package `health` implements the gRPC health checking protocol (`grpc_health_v1`). `Handler` accepts the `HealthWatcher` contract, implemented by
`observability/health.Coordinator`, and supports `Check` (single service), `List` (all services), and `Watch` (server-streaming status updates).
The watcher may be shared concurrently with HTTP health endpoints and readiness probes. Duplicate consecutive statuses are suppressed on Watch streams.

## Key types

| Type / Interface | Description                                                                            |
|------------------|----------------------------------------------------------------------------------------|
| `Handler`        | Implements `grpc_health_v1.HealthServer` by delegating to a shared `Coordinator`       |
| `HealthWatcher`  | Reports and subscribes to health status changes                                       |
| `ToProto`        | Converts a `health.ServingStatus` to the `grpc_health_v1` serving status               |

## RPCs

| Method   | Type             | Description                                                                |
|----------|------------------|----------------------------------------------------------------------------|
| `Check`  | Unary            | Returns serving status for a named service or overall server status        |
| `List`   | Unary            | Returns a map of all registered services and their serving statuses        |
| `Watch`  | Server-streaming | Pushes status changes until client disconnect or server shutdown           |
