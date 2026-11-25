# server

```go
import "github.com/altessa-s/go-atlas/internal/transport/server"
```

Listener and lifecycle management shared by the HTTP and gRPC server implementations.

## Key functions

| Function | Purpose |
|----------|---------|
| `BaseServer.Start` | Runs the protocol-specific serve callback and monitors it for failures. |
| `BaseServer.GracefulShutdown` | Invokes protocol shutdown and reports completion through `Done`. |
| `BaseServer.Err` | Returns the recorded serve error. Lifecycle logs expose a fixed classification. |
