# tracing

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"
```

Package `tracing` provides gRPC interceptors for distributed tracing. Automatic span creation, W3C Trace Context propagation via gRPC metadata,
and span attributes for method, service, status code, and peer address. Server and client interceptors for both unary and streaming RPCs.

## Span attributes

| Attribute Key          | Description                                        |
|------------------------|----------------------------------------------------|
| `rpc.system`           | Always "grpc"                                      |
| `rpc.service`          | Service name extracted from method path             |
| `rpc.method`           | Method name extracted from method path              |
| `rpc.grpc.status_code` | gRPC status code (0 = OK)                          |
| `net.peer.name`        | Peer address (server: client IP, client: target)    |

## Options

| Option               | Default              | Description                                         |
|----------------------|----------------------|-----------------------------------------------------|
| `WithPropagator`     | W3C Trace Context    | Custom trace context propagator                     |
| `WithSpanNameFunc`   | full method path     | Custom function for generating span names            |
| `WithIgnoreMethods`  | --                   | Methods to skip tracing                              |
| `WithIgnorePatterns` | --                   | Regex patterns for methods to skip                   |
| `WithLogger`         | discard              | Structured logger                                    |
