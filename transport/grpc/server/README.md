# server

```go
import "github.com/altessa-s/go-atlas/transport/grpc/server"
```

Package `server` (imported as `grpc`) provides a production-ready gRPC server wrapper with graceful shutdown, TLS support, and reflection. Create a
`Server` with `New`, register services via `RegisterHandlers`, optionally add interceptors with `RegisterInterceptors`, then call `Start`. The
`GRPCServer` interface defines the full contract. All exported methods are safe for concurrent use after startup.

Each call to `RegisterHandlers` appends handlers. Implement `Register(grpc.ServiceRegistrar, <-chan struct{})` to submit service descriptors at startup.
`Start` validates the descriptors before creating the gRPC server and returns an error matching `ErrInvalidServiceRegistration` for invalid registrations.

## Features

| Feature                | Description                                                                        |
|------------------------|------------------------------------------------------------------------------------|
| Lifecycle management   | Graceful shutdown with configurable timeouts via `Shutdown`                         |
| Security               | Integrated TLS support with automated certificate management                       |
| Observability          | Built-in support for logging, metrics (Prometheus), and distributed tracing        |
| Extensibility          | Custom service registration via `Handler` and interceptors via `ServerInterceptor`  |

## Key types

| Type / Interface | Description                                                                            |
|------------------|----------------------------------------------------------------------------------------|
| `GRPCServer`     | Lifecycle observation, startup, shutdown, and handler and interceptor registration  |
| `Server`         | Concrete implementation wrapping `grpc.Server` with lifecycle and TLS management       |
| `Handler`        | Interface for service handlers that submit descriptors through a `grpc.ServiceRegistrar` |

## Options

| Option              | Default  | Description                                                                   |
|---------------------|----------|-------------------------------------------------------------------------------|
| `WithAddress`, `WithName`, `WithLogger` | :0, empty, discard | TCP listen address and lifecycle logging |
| `WithTLSConfig` | nil | TLS settings for the listener |
| `WithLifecycleTimeouts` | See below | Startup verification, graceful shutdown limit, and warning interval |
| `WithGrpcOptions`   | --       | Appends `grpc.ServerOption` values in call order                               |
| `WithReflection`    | false    | Enables the gRPC server reflection service for tooling and debugging          |
| `WithListener`      | --       | Serve this `net.Listener` (e.g. bufconn in tests) instead of listening on the address |
| `WithStartupVerification` | 2s | How long `Start` waits for an early serve error; 0 skips the wait (tests)       |

## Subpackages

| Package                                           | Description                                                             |
|---------------------------------------------------|-------------------------------------------------------------------------|
| [factory](../../../factory/transport/grpc/server) | Configuration-based server and interceptor creation from config structs |

## TLS listeners

With server TLS configured with `WithTLSConfig`, pass a raw listener if
you supply one. The protocol server negotiates TLS for both supplied and
automatically created listeners.

## Lifecycle configuration

Pass [`lifecycle.Timeouts`](../../lifecycle) to `WithLifecycleTimeouts`. Start with `lifecycle.DefaultTimeouts()` to override individual fields.
`LifecycleTimeouts()` returns the effective settings. The shorter of the graceful shutdown limit and the caller's deadline bounds protocol cleanup.
A second `Start` on a running server returns `ErrServerAlreadyStarted`. Both lifecycle errors support `errors.Is`.

## Lifecycle observation

`Done() <-chan struct{}` closes when shutdown is observed or a post-start Serve
failure occurs. After it closes, `Err() error` reports the first unexpected Serve
error, or nil for normal shutdown. Call `Shutdown` to finish protocol cleanup even
after a Serve failure.

Once `Done` closes, `Start` returns `ErrServerClosed`, including after a post-start
Serve failure. Create a new server to restart. Early startup errors are returned
by `Start` and leave `Done` open; a supplied listener may be consumed by a failed
Serve, so retrying with that listener may not succeed.
