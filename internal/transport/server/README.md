# server

```go
import "github.com/altessa-s/go-atlas/internal/transport/server"
```

Listener and lifecycle management shared by the HTTP and gRPC server implementations.

## Key functions

| Function | Purpose |
|----------|---------|
| `BaseServer.Start` | Runs protocol startup and returns early Serve failures directly. |
| `BaseServer.GracefulShutdown` | Runs protocol cleanup once and waits for Serve error observation to stop. |
| `BaseServer.Done` | Closes after a post-start Serve failure or lifecycle shutdown. |
| `BaseServer.Err` | Returns the post-start Serve error, including one reported during graceful shutdown. Lifecycle logs expose a fixed classification. |
