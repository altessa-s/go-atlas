# factory

```go
import "github.com/altessa-s/go-atlas/factory/transport/http/server"
```

Package `factory` provides a fluent builder for creating HTTP servers and middleware from configuration. `ServerBuilder` uses deferred error
accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
srv, err := factory.New(cfg.Http).
    UseLogger(logger).
    UseTracer(tracer).
    UseTlsProviders(tlsProviders).
    WithMiddlewares().
    Build()
```

## Middleware Control

`WithRealIPMiddleware` honors forwarding headers only from configured `trustedProxies`. Set `trustPrivatePeers: true` in its configuration to restore
implicit private-peer trust. Authentication must be installed explicitly with `WithMiddleware`; `WithMiddlewares` does not install it.

Three levels of control are available:

### Level 1: All config-based middleware at once

```go
factory.New(cfg.Http).
    UseLogger(logger).
    WithMiddlewares().
    Build()
```

### Excluding specific middleware

```go
factory.New(cfg.Http).
    UseLogger(logger).
    WithMiddlewares(corsmw.ID, limitermw.ID).
    Build()
```

### Level 2: Individual config-based middleware

```go
factory.New(cfg.Http).
    UseLogger(logger).
    WithRecoveryMiddleware().
    WithLoggerMiddleware().
    WithCorsMiddleware().
    Build()
```

### Level 3: Custom pre-built middleware

```go
factory.New(cfg.Http).
    UseLogger(logger).
    WithMiddleware(customRecovery, customLogger).
    Build()
```

### Disabling specific middleware

```go
factory.New(cfg.Http).
    UseLogger(logger).
    WithMiddlewares().
    WithoutCorsMiddleware().
    WithoutSecurityHeadersMiddleware().
    Build()
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ServerBuilder` for the given HTTP config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseTlsProviders` | Sets TLS providers for server TLS configuration |
| `UseTracer` | Sets the tracer used by the tracing middleware |
| `UseLimiter` | Sets the rate limiter used by the limiter middleware |
| `UseIdempotency` | Sets the idempotency keeper used by the idempotency middleware |
| `UseMeter`     | Sets the `metrics.Meter` (a `metrics.Collector` also fits) used by the metrics middleware (no-op when unset) |
| `UseGeoResolver` | Sets the geo resolver used by the geoacl middleware |
| `Meter` | Returns the configured `metrics.Meter`, or `metrics.Noop()` when none was set |

### Router

| Method | Description |
|--------|-------------|
| `WithRouter` | Sets a custom router (default: `router/std` over `http.ServeMux`) |
| `WithRouterPrefix` | Sets a path prefix for the default router |

### Timeouts

| Method | Description |
|--------|-------------|
| `WithReadTimeout` | Overrides the read timeout from config |
| `WithReadHeaderTimeout` | Overrides the header-read timeout from config |
| `WithWriteTimeout` | Overrides the write timeout from config |
| `WithIdleTimeout` | Overrides the idle timeout from config |

### TLS

| Method | Description |
|--------|-------------|
| `WithTLSConfig` | Sets an explicit TLS configuration, bypassing config |
| `WithoutTLS` | Disables TLS even if configured in config |

### Config-Based Middleware

| Method | Description |
|--------|-------------|
| `WithMiddlewares(exclude ...middlewares.Middleware)` | Creates all enabled middleware from config; pass typed IDs (e.g., `corsmw.ID`) to skip specific ones |
| `WithBodyLimitMiddleware` | Body size limit middleware |
| `WithCorsMiddleware` | CORS middleware |
| `WithGeoAclMiddleware` | Geographic access control middleware (requires geo resolver) |
| `WithIdempotencyMiddleware` | Idempotency middleware (requires keeper) |
| `WithIpAclMiddleware` | IP access control middleware |
| `WithLimiterMiddleware` | Rate limiter middleware (requires limiter) |
| `WithLoggerMiddleware` | Structured request logging middleware |
| `WithMetricsMiddleware` | Metrics middleware (`metrics.Meter`-backed) |
| `WithRealIPMiddleware` | Real client IP extraction middleware |
| `WithRecoveryMiddleware` | Panic recovery middleware |
| `WithRequestIDMiddleware` | Request ID extraction/generation middleware |
| `WithSecurityHeadersMiddleware` | Security headers middleware |
| `WithTracingMiddleware` | Distributed tracing middleware (requires tracer) |

`WithGeoAclMiddleware` maps `geoACL.allowPrivateAddresses` (default `false`) to `geoacl.WithAllowPrivateAddresses`. A GeoIP database has no location
for loopback and private addresses: a rule with deny lists only leaves such a client to `defaultPolicy`, a rule with an allow list denies it. The
key lets these clients through instead. It skips the geographic check, deny lists included, for private client addresses, so it is only safe when
the client IP source is trustworthy (`realIP` with `trustedProxies`).

Policies are looked up for canonical request paths only (see
[Ignore lists and request paths](../../../../transport/http/server/middlewares/README.md#ignore-lists-and-request-paths)): a path with an empty,
`.` or `..` segment or an encoded slash (`//admin`, `/a/../admin`, `/v1%2Fadmin`) is never on an `ignorePaths` / `ignorePatterns` list, gets 403
from `ipACL` and `geoACL`, and requires an idempotency key once `idempotency.mandatoryPaths` or `mandatoryPatterns` is set.

### Disable Middleware

| Method | Description |
|--------|-------------|
| `WithoutBodyLimitMiddleware` | Disables body limit middleware |
| `WithoutCorsMiddleware` | Disables CORS middleware |
| `WithoutGeoAclMiddleware` | Disables geoacl middleware |
| `WithoutIdempotencyMiddleware` | Disables idempotency middleware |
| `WithoutIpAclMiddleware` | Disables ipacl middleware |
| `WithoutLimiterMiddleware` | Disables rate limiter middleware |
| `WithoutLoggerMiddleware` | Disables logger middleware |
| `WithoutPrometheusMiddleware` | Disables the Prometheus metrics middleware |
| `WithoutRealIPMiddleware` | Disables real IP middleware |
| `WithoutRecoveryMiddleware` | Disables recovery middleware |
| `WithoutRequestIDMiddleware` | Disables request ID middleware |
| `WithoutSecurityHeadersMiddleware` | Disables security headers middleware |
| `WithoutTracingMiddleware` | Disables tracing middleware |

### Custom Middleware

| Method | Description |
|--------|-------------|
| `WithMiddleware` | Adds pre-built middleware (sorted and deduplicated with config MW) |
| `WithRawMiddleware` | Adds raw middleware functions (bypasses sorting) |

### Handlers

| Method | Description |
|--------|-------------|
| `WithHandler` | Adds custom handlers |
| `WithoutBuiltinHandlers` | Disables all built-in handlers (ping, healthz, readyz, pprof, metrics) |
| `WithPprof` | Enables pprof handlers, overriding config |
| `WithoutPprof` | Disables pprof handlers |
| `WithoutMetrics` | Disables Prometheus metrics handler |
| `WithInternalPrefix` | Overrides the built-in handler prefix (default `/internal`) |

### Address

| Method | Description |
|--------|-------------|
| `WithListenAddress` | Overrides the listen address from config |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the server |

## Request body cap

`Config.MaxBodySize` overrides the `Writer.Read` cap (negative disables it). When zero, an installed, enabled bodyLimit middleware supplies its positive
MaxSize; otherwise the writer uses 4 MiB. Uninstalled or disabled middleware does not raise this default. Middleware limits still apply independently.
Direct body reads require middleware or their own limit.
