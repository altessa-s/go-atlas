# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/geoacl"
```

Package `geoacl` provides gRPC interceptors for geographic access control.
It enforces allow and deny rules per endpoint based on the client's geographic location.

## Usage

```go
registry := geoacl.NewRegistry(geoacl.PolicyDeny)
registry.Register("/admin.AdminService/Delete", &geoacl.AccessRule{
    AllowCountries: []string{"US", "CA"},
})

interceptor := geoacl.ServerInterceptor(resolver, registry,
    geoacl.WithFallbackBehavior(fallback.Deny),
    geoacl.WithLogger(logger),
)

server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(interceptor.Unary()),
    grpc.ChainStreamInterceptor(interceptor.Stream()),
)
```

## Options

| Option | Description |
|--------|-------------|
| `WithFallbackBehavior` | Action when geo resolution fails (default: deny) |
| `WithLogger` | Logger for debug output |
| `WithMetrics` | Metrics collector for access decisions |

## Key Types

| Type | Description |
|------|-------------|
| `Registry` | Manages per-endpoint geographic access rules |
| `AccessRule` | Defines allow/deny lists for countries |
| `GeoResolver` | Interface for IP-to-country resolution |
| `ServerInterceptor` | Combined unary/stream interceptor |

## Features

- Per-endpoint geographic access control
- Support for both allow and deny lists
- Configurable fallback behavior
- Integration with real IP detection
- Metrics for monitoring access patterns