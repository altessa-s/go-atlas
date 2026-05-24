# ipacl

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
```

Package `ipacl` provides HTTP middleware for IP-based access control.
It enforces allowlist and denylist rules per URL path based on client IP addresses.

## Usage

```go
registry := ipacl.NewRegistry(ipacl.PolicyDeny)
registry.SetDefault(&ipacl.AccessRule{
    Allowlist: []netip.Prefix{
        netip.MustParsePrefix("10.0.0.0/8"),
    },
})
registry.Register("/admin/*", &ipacl.AccessRule{
    Allowlist: []netip.Prefix{
        netip.MustParsePrefix("192.168.1.0/24"),
    },
})

mw := ipacl.New(registry,
    ipacl.WithFallbackBehavior(fallback.Deny),
    ipacl.WithLogger(logger),
)

handler := mw.Handler(myHandler)
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
| `Registry` | Manages per-path IP access rules |
| `AccessRule` | Defines allowlist/denylist IP ranges |
| `Middleware` | HTTP middleware implementation |

## Features

- CIDR-based IP allowlists and denylists
- Path-pattern based access control
- IPv4 and IPv6 support
- Integration with real IP detection
- HTTP 403 response for denied requests