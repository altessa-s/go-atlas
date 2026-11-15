# ipacl

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
```

Package `ipacl` provides HTTP middleware for IP-based access control. It enforces allowlist and denylist rules per URL path based on client IP
addresses.

## Usage

The registry and rule types live in the public `transport/policy` packages. Services usually configure
the middleware through the server factory instead: YAML `middlewares.ipACL` plus `WithIpAclMiddleware`.

```go
import (
    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
    "github.com/altessa-s/go-atlas/transport/policy/fallback"
    iprules "github.com/altessa-s/go-atlas/transport/policy/ipacl"
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
| `WithIgnorePatterns` | Regex patterns for paths to skip (default: none) |

No path is exempt by default: health, metrics, and pprof endpoints are checked like any other route. To exempt the built-in endpoints, opt in
explicitly with `ipacl.WithIgnorePatterns(defaults.IgnorePatterns...)` (package [`defaults`](../defaults)); the patterns are anchored and match only
the endpoints at the root or under `/internal`.

## Request paths

Rules and ignore lists are looked up for the canonical request path only, the form a router dispatches on: a leading slash and no empty, `.` or
`..` segment; a trailing slash is significant (`/admin` and `/admin/` are different paths). A request with any other path (`//admin`,
`/public/../admin`), or with an encoded slash in it (`/v1%2Fadmin`, which a router matching the escaped path dispatches as one segment), is never
on the ignore list and is answered with 403, so no spelling of a path reaches a handler under the rule of another path. See
[Ignore lists and request paths](../README.md#ignore-lists-and-request-paths) for what each router does with such requests.

## Key Types

`Registry`, `AccessRule` and `NewRegistry` are defined in `transport/policy/ipacl` package, `fallback.Behavior` in
`transport/policy/fallback`.

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