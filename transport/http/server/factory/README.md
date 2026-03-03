# factory

```go
import "github.com/altessa-s/go-atlas/transport/http/server/factory"
```

Package `factory` provides a fluent builder for creating HTTP servers and middleware from configuration.
`ServerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

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

Three levels of control are available:

### Level 1: All config-based middleware at once

```go
factory.New(cfg.Http).
    UseLogger(logger).
    WithMiddlewares().
    Build()
```

### Level 2: Individual config-based middleware

```go
mwCfg := cfg.Http.Middlewares
factory.New(cfg.Http).
    UseLogger(logger).
    WithRecoveryMiddleware(mwCfg.Recovery).
    WithLoggerMiddleware(mwCfg.Logger).
    WithCorsMiddleware(mwCfg.Cors).
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

### Router

| Method | Description |
|--------|-------------|
| `WithRouter` | Sets a custom router (default: Gorilla) |
| `WithRouterPrefix` | Sets a path prefix for the default Gorilla router |

### Timeouts

| Method | Description |
|--------|-------------|
| `WithReadTimeout` | Overrides the read timeout from config |
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
| `WithMiddlewares` | Creates all enabled middleware from config |
| `WithBodyLimitMiddleware` | Body size limit middleware |
| `WithCorsMiddleware` | CORS middleware |
| `WithIdempotencyMiddleware` | Idempotency middleware (requires keeper) |
| `WithLimiterMiddleware` | Rate limiter middleware (requires limiter) |
| `WithLoggerMiddleware` | Structured request logging middleware |
| `WithPrometheusMiddleware` | Prometheus metrics middleware |
| `WithRealIPMiddleware` | Real client IP extraction middleware |
| `WithRecoveryMiddleware` | Panic recovery middleware |
| `WithRequestIDMiddleware` | Request ID extraction/generation middleware |
| `WithSecurityHeadersMiddleware` | Security headers middleware |
| `WithTracingMiddleware` | Distributed tracing middleware (requires tracer) |

### Disable Middleware

| Method | Description |
|--------|-------------|
| `WithoutBodyLimitMiddleware` | Disables body limit middleware |
| `WithoutCorsMiddleware` | Disables CORS middleware |
| `WithoutIdempotencyMiddleware` | Disables idempotency middleware |
| `WithoutLimiterMiddleware` | Disables rate limiter middleware |
| `WithoutLoggerMiddleware` | Disables logger middleware |
| `WithoutPrometheusMiddleware` | Disables Prometheus middleware |
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
| `WithoutPprof` | Disables pprof handlers |
| `WithoutMetrics` | Disables Prometheus metrics handler |

### Address

| Method | Description |
|--------|-------------|
| `WithListenAddress` | Overrides the listen address from config |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the server |
