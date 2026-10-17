# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
```

Package `geoacl` provides HTTP middleware for geographic access control. It enforces allow and deny rules per URL path based on the client's geographic
location.

## Usage

The registry and rule types live in module-internal packages, so direct wiring like the snippet below compiles only inside go-atlas. The server factory
builds the registry from YAML `middlewares.geoACL` (`WithGeoAclMiddleware`) and takes the resolver from `UseGeoResolver`; because `GeoResolver.Resolve`
returns the internal `GeoInfo` type, resolvers can currently be implemented only inside the module.

```go
import (
    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
    "github.com/altessa-s/go-atlas/transport/internal/fallback"
    georules "github.com/altessa-s/go-atlas/transport/internal/geoacl"
)

registry := georules.NewRegistry(georules.PolicyDeny)
registry.SetDefault(&georules.AccessRule{
    AllowContinents: []string{"EU", "NA"},
})
registry.RegisterPattern(regexp.MustCompile(`^/admin/`), &georules.AccessRule{
    AllowCountries: []string{"US", "CA"},
})

mw := geoacl.New(resolver, registry,
    geoacl.WithFallbackBehavior(fallback.Deny),
    geoacl.WithLogger(logger),
)

handler := mw.Handler(myHandler)
```

## Options

| Option | Description |
|--------|-------------|
| `WithFallbackBehavior` | Action when geo resolution fails (default: deny) |
| `WithLogger` | Logger for debug output |
| `WithIgnorePaths` | Paths to skip |
| `WithIgnorePatterns` | Regex patterns for paths to skip (default: health, metrics, pprof) |

## Key Types

`Registry`, `AccessRule`, `GeoResolver` and `NewRegistry` are defined in the module-internal `transport/internal/geoacl` package, `fallback.Behavior` in
`transport/internal/fallback`.

| Type | Description |
|------|-------------|
| `Registry` | Manages per-path geographic access rules |
| `AccessRule` | Defines allow/deny lists for countries and continents |
| `GeoResolver` | Interface for IP-to-location resolution |
| `New` / `Middleware` | Constructors returning the middleware or a `func(http.Handler) http.Handler` |

## Features

- Path-pattern based geographic access control
- Support for country and continent level rules
- Configurable fallback behavior
- Integration with real IP detection
- HTTP 403 response for denied requests