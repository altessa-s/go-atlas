# interceptors

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors"
```

Package `interceptors` provides a unified framework for gRPC interceptors with automatic dependency ordering via topological sort. Core abstractions
include Chain, BaseInterceptor, ServerInterceptor/ClientInterceptor interfaces, Error type, and the Driven Interceptor pattern.

## Key types

| Type / Interface          | Description                                                                              |
|---------------------------|------------------------------------------------------------------------------------------|
| `Chain`                   | Collects interceptors, resolves dependency order, produces `grpc.ServerOption` / `DialOption` |
| `BaseInterceptor`         | Embeddable base with endpoint filtering, logging, and name identification                |
| `ServerInterceptor`       | Interface for server-side interception of unary and streaming RPCs                       |
| `ClientInterceptor`       | Interface for client-side interception of unary and streaming RPCs                       |
| `Interceptor`             | Minimal contract (`Name`) required by every interceptor in a Chain                       |
| `Ref`                     | Creates a lightweight `Interceptor` from a name string, used for typed `ID` exports      |
| `Error`                   | Combines `grpc/status.Status` with a standard Go error, implements both interfaces       |
| `Matcher` / `MatchFunc`   | Dynamic runtime condition for conditional interceptor activation                         |
| `DrivenInterceptorFunc`   | Function adapter for the driven interceptor pattern                                      |

## Chain methods

| Method             | Description                                                                   |
|--------------------|-------------------------------------------------------------------------------|
| `NewChain`         | Creates a new chain from any combination of server/client interceptors        |
| `ServerOptions`    | Returns `[]grpc.ServerOption` with topological ordering, auto-prepends metadata |
| `ClientOptions`    | Returns `[]grpc.DialOption` with topological ordering, auto-prepends metadata |
| `DependencyOrder`  | Returns computed interceptor ordering for debugging                           |
| `DependencyGraph`  | Returns map of interceptor names to declared dependencies                     |
| `WithLogger`       | Sets logger for dependency resolution debug output                            |

## Interceptor Identity

Every interceptor sub-package exports three identification helpers:

| Export      | Type                | Purpose                                                              |
|-------------|---------------------|----------------------------------------------------------------------|
| `Name()`    | `func() string`     | Returns the interceptor name as a plain string                       |
| `ID`        | `Interceptor`       | Lightweight typed reference for factory exclusion lists (`auth.ID`)  |
| Dependencies | `[]string`         | Uses sibling `Name()` calls instead of string literals               |

## Stream hooks

Driven interceptors receive per-message stream hooks through `ServerStreamWrapper` / `ClientStreamWrapper`: `PostMsgReceive` after a receive,
the optional `PreMsgSend` (`driver.DriverStreamPreSend`) before a message reaches the transport, and `PostMsgSent` after it is sent. A
wrapper is reused only for a context update or when it carries no driver yet; every further driver gets its own nested wrapper, so each
driven interceptor in a chain sees every stream message. On a server, pre-send hooks run from the interceptor closest to the handler
outwards; on a client, from the outermost interceptor inwards.

Earlier releases reused the first wrapper and dropped later drivers, so in the default server chain driven interceptors such as
`fieldbehavior`, `fieldmask` and `protovalidator` (and client `errstatus`) silently received no stream message hooks. They now apply to every
streamed message: streamed requests are validated and sanitized, and streamed responses are sanitized and masked before they are sent.

## Subpackages

| Package                                        | Description                                          |
|------------------------------------------------|------------------------------------------------------|
| [audit](./audit)                               | Automatic request auditing via driven pattern         |
| [auth](./auth)                                 | Token-based authentication and scope authorization    |
| [cache](./cache)                               | Response caching with compression support             |
| [defaults](./defaults)                         | Shared default values for interceptor configuration   |
| [driver](./driver)                             | Core interfaces for the driven interceptor pattern    |
| [errstatus](./errstatus)                       | Error-to-status and status-to-error conversion        |
| [fieldbehavior](./fieldbehavior)               | AIP-203 `field_behavior` strip on request/response    |
| [fieldmask](./fieldmask)                       | AIP-134 `update_mask` + AIP-157 `read_mask` enforcement |
| [geoacl](./geoacl)                             | Country/region-based access control on caller IP      |
| [health](./health)                             | Service health checking                               |
| [idempotency](./idempotency)                   | Server interceptor for idempotent request handling, plus client-side helpers and `UnaryClientInterceptor` |
| [ipacl](./ipacl)                               | IP/CIDR allow- and deny-list enforcement              |
| [limiter](./limiter)                           | Rate limiting with standard response headers          |
| [logger](./logger)                             | Request and response logging                          |
| [metadata](./metadata)                         | Call metadata extraction and context injection         |
| [metrics](./metrics)                           | Prometheus metrics collection                         |
| [protovalidator](./protovalidator)             | Protocol buffer message validation                    |
| [realip](./realip)                             | Real client IP extraction from proxy headers          |
| [recovery](./recovery)                         | Panic recovery with stack trace capture               |
| [requestid](./requestid)                       | Request ID generation and propagation                 |
| [tracing](./tracing)                           | Distributed tracing with W3C Trace Context            |
