# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/geoacl"
```

Package `geoacl` provides gRPC interceptors for geographic access control. It enforces allow and deny rules per endpoint based on the client's
geographic location.

## Usage

```go
// import geopolicy "github.com/altessa-s/go-atlas/transport/policy/geoacl"
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
| `WithIgnorePatterns` | Regex patterns for methods to skip (default: none) |
| `WithAllowPrivateAddresses` | Let loopback, private and link-local clients through without a geographic check (default: off) |

No method is exempt by default, gRPC health and reflection included: a denied IP or country is denied on every method. Exempt only what must be
public, with exact full method names or anchored patterns; the reflection and health exemptions applied by default before are an explicit
opt-in: `geoacl.WithIgnorePatterns(defaults.IgnorePatterns...)` (package [`defaults`](../defaults)).

## Internal callers

A GeoIP database has no location for loopback, private and link-local addresses, so health probes and in-cluster callers arrive with an unknown
location. A rule with deny lists only under `PolicyAllow` lets them through; a rule with an allow list, or a `PolicyDeny` registry, denies them (see
[`transport/policy/geoacl`](../../../../transport/policy/geoacl)). Nothing is exempted silently. Choose one of the two explicit controls:

- by method: `WithIgnoreMethods` / `WithIgnorePatterns` for the methods internal callers use (health);
- by caller: `WithAllowPrivateAddresses()` skips the geographic check, deny lists included, when the client address is loopback, private,
  link-local or otherwise reserved (`netipx.IsPrivate`).

The client address is the one `realip` resolved. With `WithAllowPrivateAddresses` forwarded metadata naming a private address is as good as a
pass, so accept it from trusted proxies only.

## Key Types

`Registry`, `AccessRule`, `GeoResolver` and `NewRegistry` come from the
[`transport/policy/geoacl`](../../../../transport/policy/geoacl) package; services outside the module configure them through the [server
factory](../../../../factory/transport/grpc/server).

| Type | Description |
|------|-------------|
| `Evaluator` | Accepts a policy registry or a custom implementation of `Evaluate` |
| `Registry` | Manages per-endpoint geographic access rules |
| `AccessRule` | Defines allow/deny lists for continents, countries, and regions |
| `GeoResolver` | Interface for IP-to-country resolution |
| `ServerInterceptor` | Combined unary/stream interceptor |

## Features

- Per-endpoint geographic access control
- Support for both allow and deny lists
- Configurable fallback behavior
- Integration with real IP detection
