# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
```

Package `geoacl` provides HTTP middleware for geographic access control. It enforces allow and deny rules per URL path based on the client's geographic
location.

## Usage

The server factory builds the registry from YAML `middlewares.geoACL` and takes the resolver from `UseGeoResolver`. Custom resolvers return
`transport/policy/geoacl.GeoInfo`.

```go
import (
    "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
    "github.com/altessa-s/go-atlas/transport/policy/fallback"
    georules "github.com/altessa-s/go-atlas/transport/policy/geoacl"
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
| `WithIgnorePatterns` | Regex patterns for paths to skip (default: none) |
| `WithAllowPrivateAddresses` | Let loopback, private and link-local clients through without a geographic check (default: off) |

No path is exempt by default: health, metrics, and pprof endpoints are checked like any other route. To exempt the built-in endpoints, opt in
explicitly with `geoacl.WithIgnorePatterns(defaults.IgnorePatterns...)` (package [`defaults`](../defaults)); the patterns are anchored and match only
the endpoints at the root or under `/internal`.

## Internal callers

A GeoIP database has no location for loopback, private and link-local addresses, so kubelet probes and in-cluster callers arrive with an unknown
location. A rule with deny lists only under `PolicyAllow` lets them through; a rule with an allow list, or a `PolicyDeny` registry, denies them (see
[`transport/policy/geoacl`](../../../../policy/geoacl)). Nothing is exempted silently. Choose one of the two explicit controls:

- by endpoint: `WithIgnorePaths` / `WithIgnorePatterns` for the routes internal callers use (health, metrics);
- by caller: `WithAllowPrivateAddresses()` skips the geographic check, deny lists included, when the client address is loopback, private,
  link-local or otherwise reserved (`netipx.IsPrivate`).

The client address is the one `realip` resolved. With `WithAllowPrivateAddresses` a forwarded header naming a private address is as good as a pass,
so accept forwarded headers from trusted proxies only.

## Request paths

Rules and ignore lists are looked up for the canonical request path only, the form a router dispatches on: a leading slash and no empty, `.` or
`..` segment; a trailing slash is significant (`/admin` and `/admin/` are different paths). A request with any other path (`//admin`,
`/public/../admin`), or with an encoded slash in it (`/v1%2Fadmin`, which a router matching the escaped path dispatches as one segment), is never
on the ignore list and is answered with 403, so no spelling of a path reaches a handler under the rule of another path. See
[Ignore lists and request paths](../README.md#ignore-lists-and-request-paths) for what each router does with such requests.

## Key Types

`Registry`, `AccessRule`, `GeoResolver` and `NewRegistry` are defined in `transport/policy/geoacl` package, `fallback.Behavior` in
`transport/policy/fallback`.

| Type | Description |
|------|-------------|
| `Evaluator` | Accepts a policy registry or a custom implementation of `Evaluate` |
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