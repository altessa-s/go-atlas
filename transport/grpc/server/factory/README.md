# factory

```go
import "github.com/altessa-s/go-atlas/transport/grpc/server/factory"
```

Package `factory` provides a fluent builder for creating a gRPC `Server` and its interceptors from configuration.
`ServerBuilder` uses deferred error accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
srv, err := factory.New(cfg.Grpc).
    UseLogger(logger).
    UseTracer(tracer).
    UseLimiter(limiter).
    UseAuth(authFn, nil).
    WithInterceptors().
    Build()
```

## Methods

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ServerBuilder` for the given gRPC config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseTlsProviders` | Sets the TLS providers used for server TLS configuration |
| `UseCacheMetadataProcessor` | Sets the metadata processor forwarded to the cache interceptor |
| `UseTracer` | Sets the tracer used by the tracing interceptor |
| `UseLimiter` | Sets the rate limiter used by the limiter interceptor |
| `UseIdempotency` | Sets the idempotency keeper used by the idempotency interceptor |
| `UseCacher` | Sets the cacher used by the cache interceptor |
| `UseAuth(authFn, clientAuth)` | Sets the authentication function and optional client auth handler |
| `UseHealthChecker` | Sets the health checker used by the health interceptor |

### Configuration

| Method | Description |
|--------|-------------|
| `WithInterceptors()` | Creates all enabled interceptors from `cfg.Interceptors` and registers them in order |
| `WithLoggerInterceptor()` | Adds a structured request/response logging interceptor |
| `WithPrometheusInterceptor()` | Adds a Prometheus metrics collection interceptor |
| `WithTracingInterceptor()` | Adds a distributed tracing interceptor |
| `WithRealIPInterceptor()` | Adds a real client IP extraction interceptor |
| `WithRecoveryInterceptor()` | Adds a panic recovery interceptor |
| `WithRequestIDInterceptor()` | Adds a request ID propagation/generation interceptor |
| `WithLimiterInterceptor()` | Adds a rate limiting interceptor |
| `WithIdempotencyInterceptor()` | Adds an idempotency enforcement interceptor |
| `WithCacheInterceptor()` | Adds a response caching interceptor |
| `WithAuthInterceptor()` | Adds an authentication interceptor |
| `WithHealthInterceptor()` | Adds a health gate interceptor |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the gRPC server with all registered interceptors |
