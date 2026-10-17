# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/geoacl"
```

Package `geoacl` provides gRPC interceptors for geographic access control. It enforces allow and deny rules per endpoint based on the client's
geographic location.

## Usage

```go
// Inside this module only; external services configure rules through the server factory.
// import geopolicy "github.com/altessa-s/go-atlas/transport/internal/geoacl"
registry := geopolicy.NewRegistry(geopolicy.PolicyDeny)
registry.Register("/admin.AdminService/Delete", &geopolicy.AccessRule{
    AllowCountries: []string{"US", "CA"},
})

interceptor := geoacl.ServerInterceptor(resolver, registry,
    geoacl.WithFallbackBehavior(fallback.Deny),
    geoacl.WithLogger(logger),
)

server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(interceptor.ServerUnaryInterceptor()),
    grpc.ChainStreamInterceptor(interceptor.ServerStreamInterceptor()),
)
```

## Options

| Option | Description |
|--------|-------------|
| `WithFallbackBehavior` | Action when geo resolution fails (default: deny) |
| `WithLogger` | Logger for debug output |
| `WithIgnoreMethods` | Exact full method names to skip |
| `WithIgnorePatterns` | Regex patterns for methods to skip (default: reflection and health) |

## Key Types

`Registry`, `AccessRule`, `GeoResolver` and `NewRegistry` come from the module-internal [`transport/internal/geoacl`](../../../internal/geoacl) package;
services outside the module configure them through the [server factory](../../server/factory).

| Type | Description |
|------|-------------|
| `Registry` | Manages per-endpoint geographic access rules |
| `AccessRule` | Defines allow/deny lists for continents, countries, and regions |
| `GeoResolver` | Interface for IP-to-country resolution |
| `ServerInterceptor` | Combined unary/stream interceptor |

## Features

- Per-endpoint geographic access control
- Support for both allow and deny lists
- Configurable fallback behavior
- Integration with real IP detection
