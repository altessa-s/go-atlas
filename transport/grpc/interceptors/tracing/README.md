# tracing

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"
```

Package `tracing` provides gRPC interceptors for distributed tracing. Automatic span creation, W3C Trace Context propagation via gRPC metadata, and span
attributes for method, service, status code, and peer address. Server and client interceptors for both unary and streaming RPCs.

## Span attributes

| Attribute Key          | Description                                        |
|------------------------|----------------------------------------------------|
| `rpc.system`           | Always "grpc"                                      |
| `rpc.service`          | Service name extracted from method path             |
| `rpc.method`           | Method name extracted from method path              |
| `rpc.grpc.status_code` | gRPC status code (0 = OK)                          |
| `net.peer.name`        | Peer address (server: client IP, client: target)    |

## RPC status and exceptions

Every completed RPC records its final numeric code in `rpc.grpc.status_code`. On a server span, `Unknown`, `DeadlineExceeded`,
`Unimplemented`, `Internal`, `Unavailable` and `DataLoss` set the span status to Error. Other non-OK server codes leave it Unset.
On a client span every non-OK code sets Error. Successful RPCs retain the toolkit's OK status.

Server exception events record the pre-conversion Go error type and message, including for a domain rejection that leaves the span Unset.
The message omits URL user information and is capped at 1024 bytes. An Error span's status description comes from the final gRPC message.

## Options

| Option               | Default              | Description                                         |
|----------------------|----------------------|-----------------------------------------------------|
| `WithPropagator`     | W3C Trace Context    | Custom trace context propagator                     |
| `WithSpanNameFunc`   | full method path     | Custom function for generating span names            |
| `WithIgnoreMethods`  | --                   | Methods to skip tracing                              |
| `WithIgnorePatterns` | --                   | Regex patterns for methods to skip                   |
| `WithLogger`         | discard              | Structured logger                                    |

## Diagnostics

By default spans record the gRPC code and status category. Use `WithRecordClientDetails(true)` for the server peer address and `WithRecordRawErrors(true)` for exception messages and status descriptions.
