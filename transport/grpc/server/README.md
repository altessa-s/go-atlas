# server

```go
import "github.com/altessa-s/go-atlas/transport/grpc/server"
```

Package `server` (imported as `grpc`) provides a production-ready gRPC server wrapper with graceful shutdown, TLS support, and reflection. Create
a `Server` with `New`, register services via `RegisterHandlers`, optionally add interceptors with `RegisterInterceptors`, then call `Start`. The
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

## Subpackages

| Package                    | Description                                                              |
|----------------------------|--------------------------------------------------------------------------|
| [factory](./factory)       | Configuration-based server and interceptor creation from config structs  |
