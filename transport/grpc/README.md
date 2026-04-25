# grpc

```go
import "github.com/altessa-s/go-atlas/transport/grpc"
```

Package `grpc` provides gRPC server and client utilities for go-atlas services. Includes connection management, interceptor chaining with automatic
dependency ordering, service handlers, and configuration-driven server creation.

## Subpackages

| Package                          | Description                                                                   |
|----------------------------------|-------------------------------------------------------------------------------|
| [client](./client)               | Generic gRPC client with connection pooling, retry, and error handling        |
| [handlers](./handlers)           | Service handlers (health checking, scheduler) implementing the Handler iface  |
| [interceptors](./interceptors)   | Middleware chain for logging, auth, metrics, caching, recovery, and more      |
| [server](./server)               | gRPC server lifecycle with graceful shutdown, TLS, and reflection             |
