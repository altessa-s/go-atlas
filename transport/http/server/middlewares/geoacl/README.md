# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
```

Package `geoacl` provides HTTP middleware for geographic access control.
It enforces allow and deny rules per URL path based on the client's geographic location.

## Usage

```go
registry := geoacl.NewRegistry(geoacl.PolicyDeny)
registry.SetDefault(&geoacl.AccessRule{
    AllowContinents: []string{"EU", "NA"},
})
registry.Register("/admin/*", &geoacl.AccessRule{
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
| `WithMetrics` | Metrics collector for access decisions |

## Key Types

| Type | Description |
|------|-------------|
| `Registry` | Manages per-path geographic access rules |
| `AccessRule` | Defines allow/deny lists for countries and continents |
| `GeoResolver` | Interface for IP-to-location resolution |
| `Middleware` | HTTP middleware implementation |

## Features

- Path-pattern based geographic access control
- Support for country and continent level rules
- Configurable fallback behavior
- Integration with real IP detection
- HTTP 403 response for denied requests