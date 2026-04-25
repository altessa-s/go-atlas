# driver

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
```

Package `driver` defines core interfaces for the driven interceptor pattern. Decouples interceptor business logic from gRPC-specific function
signatures by delegating to a request-scoped `Driver` with PreCall/PostCall hooks. `DriverStream` extends the pattern with per-message hooks.

## Key types

| Type / Interface     | Description                                                                  |
|----------------------|------------------------------------------------------------------------------|
| `Driver`             | Request-scoped lifecycle hooks: PreCall (before handler) and PostCall (after) |
| `DriverStream`       | Extends Driver with PostMsgReceive and PostMsgSent for streaming RPCs        |
| `DrivenInterceptor`  | Entry point: returns a request-scoped Driver and modified context             |
| `StreamType`         | Identifies streaming direction: None, Client, Server, Bidi                   |

## Functions

| Function       | Description                                              |
|----------------|----------------------------------------------------------|
| `NoopDriver`   | Returns a no-op Driver for tests or passthrough calls    |
