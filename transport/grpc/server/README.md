# server

```go
import "github.com/altessa-s/go-atlas/transport/grpc/server"
```

Package `server` (imported as `grpc`) provides a production-ready gRPC server wrapper with graceful shutdown, TLS support, and reflection. Create a
`Server` with `New`, register services via `RegisterHandlers`, optionally add interceptors with `RegisterInterceptors`, then call `Start`. The
`GRPCServer` interface defines the full contract. All exported methods are safe for concurrent use after startup.

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
| `GRPCServer`     | Full server contract: extends base `Server` with handler and interceptor registration  |
| `Server`         | Concrete implementation wrapping `grpc.Server` with lifecycle and TLS management       |
| `Handler`        | Interface for service handlers: `Register(gs, stop)` attaches services to the server   |

## Options

| Option              | Default  | Description                                                                   |
|---------------------|----------|-------------------------------------------------------------------------------|
| `WithBaseOptions`   | --       | Base server options: address, TLS config, logger, timeouts                    |
| `WithGrpcOptions`   | --       | Additional `grpc.ServerOption` values passed to the underlying gRPC server    |
| `WithReflection`    | false    | Enables the gRPC server reflection service for tooling and debugging          |
| `WithListener`      | --       | Serve this `net.Listener` (e.g. bufconn in tests) instead of listening on the address |
| `WithStartupVerification` | 2s | How long `Start` waits for an early serve error; 0 skips the wait (tests)       |

## Subpackages

| Package                    | Description                                                              |
|----------------------------|--------------------------------------------------------------------------|
| [factory](./factory)       | Configuration-based server and interceptor creation from config structs  |

## Lifecycle observation

`Done() <-chan struct{}` closes when shutdown is observed or a post-start Serve
failure occurs. After it closes, `Err() error` reports the first unexpected Serve
error, or nil for normal shutdown. Call `Shutdown` to finish protocol cleanup even
after a Serve failure.

Once `Done` closes, `Start` returns `ErrServerClosed`, including after a post-start
Serve failure. Create a new server to restart. Early startup errors are returned
by `Start` and leave `Done` open; a supplied listener may be consumed by a failed
Serve, so retrying with that listener may not succeed.

`GRPCServer` gained `Done() <-chan struct{}` and `Err() error` through its embedded
base interface. This is an interface addition: custom implementations and mocks
must implement both methods.
