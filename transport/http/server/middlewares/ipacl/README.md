# ipacl

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
```

Package `ipacl` provides HTTP middleware for IP-based access control. It enforces allowlist and denylist rules per URL path based on client IP
addresses.

## Usage

The registry and rule types live in module-internal packages, so direct wiring like the snippet below compiles only inside go-atlas. Services configure
the middleware through the server factory instead: YAML `middlewares.ipACL` plus `WithIpAclMiddleware`.

```go
import (
    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
    "github.com/altessa-s/go-atlas/transport/internal/fallback"
    iprules "github.com/altessa-s/go-atlas/transport/internal/ipacl"
)

registry := iprules.NewRegistry(iprules.PolicyDeny)
registry.SetDefault(&iprules.AccessRule{
    Allowlist: []netip.Prefix{
        netip.MustParsePrefix("10.0.0.0/8"),
    },
})
registry.RegisterPattern(regexp.MustCompile(`^/admin/`), &iprules.AccessRule{
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
| `WithIgnorePaths` | Paths to skip |
| `WithIgnorePatterns` | Regex patterns for paths to skip (default: health, metrics, pprof) |

## Key Types

`Registry`, `AccessRule` and `NewRegistry` are defined in the module-internal `transport/internal/ipacl` package, `fallback.Behavior` in
`transport/internal/fallback`.

| Type | Description |
|------|-------------|
| `Registry` | Manages per-path IP access rules |
| `AccessRule` | Defines allowlist/denylist IP ranges |
| `New` / `Middleware` | Constructors returning the middleware or a `func(http.Handler) http.Handler` |

## Features

- CIDR-based IP allowlists and denylists
- Path-pattern based access control
- IPv4 and IPv6 support
- Integration with real IP detection
- HTTP 403 response for denied requests