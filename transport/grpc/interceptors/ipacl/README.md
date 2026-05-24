# ipacl

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/ipacl"
```

Package `ipacl` provides gRPC interceptors for IP-based access control.
It enforces allowlist and denylist rules per endpoint based on client IP addresses.

## Usage

```go
registry := ipacl.NewRegistry(ipacl.PolicyDeny)
registry.Register("/admin.AdminService/Delete", &ipacl.AccessRule{
    Allowlist: []netip.Prefix{
        netip.MustParsePrefix("10.1.1.0/24"),
        netip.MustParsePrefix("192.168.0.0/16"),
    },
})

interceptor := ipacl.ServerInterceptor(registry,
    ipacl.WithFallbackBehavior(fallback.Deny),
    ipacl.WithLogger(logger),
)

server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(interceptor.Unary()),
    grpc.ChainStreamInterceptor(interceptor.Stream()),
)
```

## Options

| Option | Description |
|--------|-------------|
| `WithFallbackBehavior` | Action when no client IP available (default: deny) |
| `WithLogger` | Logger for debug output |
| `WithMetrics` | Metrics collector for access decisions |

## Key Types

| Type | Description |
|------|-------------|
| `Registry` | Manages per-endpoint IP access rules |
| `AccessRule` | Defines allowlist/denylist IP ranges |
| `ServerInterceptor` | Combined unary/stream interceptor |

## Features

- CIDR-based IP allowlists and denylists
- Per-endpoint access control
- IPv4 and IPv6 support
- Integration with real IP detection
- Configurable fallback behavior