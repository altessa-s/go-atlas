# ipacl

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/ipacl"
```

Package `ipacl` provides gRPC interceptors for IP-based access control. It enforces allowlist and denylist rules per endpoint based on client IP
addresses.

## Usage

```go
// import aclpolicy "github.com/altessa-s/go-atlas/transport/policy/ipacl"
registry := aclpolicy.NewRegistry(aclpolicy.PolicyDeny)
registry.Register("/admin.AdminService/Delete", &aclpolicy.AccessRule{
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
    grpc.ChainUnaryInterceptor(interceptor.ServerUnaryInterceptor()),
    grpc.ChainStreamInterceptor(interceptor.ServerStreamInterceptor()),
)
```

## Options

| Option | Description |
|--------|-------------|
| `WithFallbackBehavior` | Action when no client IP available (default: deny) |
| `WithLogger` | Logger for debug output |
| `WithIgnoreMethods` | Exact full method names to skip |
| `WithIgnorePatterns` | Regex patterns for methods to skip (default: none) |

No method is exempt by default, gRPC health and reflection included: a denied IP is denied on every method. Exempt only what must be
public, with exact full method names or anchored patterns; the reflection and health exemptions applied by default before are an explicit
opt-in: `ipacl.WithIgnorePatterns(defaults.IgnorePatterns...)` (package [`defaults`](../defaults)).

## Key Types

`Registry`, `AccessRule` and `NewRegistry` come from the [`transport/policy/ipacl`](../../../../transport/policy/ipacl) package;
services outside the module configure them through the [server factory](../../../../factory/transport/grpc/server).

| Type | Description |
|------|-------------|
| `Evaluator` | Accepts a policy registry or a custom implementation of `Evaluate` |
| `Registry` | Manages per-endpoint IP access rules |
| `AccessRule` | Defines allowlist/denylist IP ranges |
| `ServerInterceptor` | Combined unary/stream interceptor |

## Features

- CIDR-based IP allowlists and denylists
- Per-endpoint access control
- IPv4 and IPv6 support
- Integration with real IP detection
- Configurable fallback behavior
