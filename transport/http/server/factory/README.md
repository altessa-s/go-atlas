# factory

```go
import "github.com/altessa-s/go-atlas/transport/http/server/factory"
```

Package `factory` provides configuration-based creation of HTTP servers and middleware. The factory reads structured config objects and
produces fully wired server and middleware instances. All created components inherit the factory's logger, tracer, and TLS providers.
`CreateMiddlewaresFromConfig` returns all enabled middleware in dependency-sorted order with duplicates removed.

## Methods

| Method                                       | Description                                                     |
|----------------------------------------------|-----------------------------------------------------------------|
| `CreateServerFromConfig`                     | Creates an HTTP server with TLS support from configuration      |
| `CreateMiddlewaresFromConfig`                | Creates all enabled middleware, dependency-sorted, deduplicated  |
| `CreateBodyLimitMiddlewareFromConfig`        | Body size limit middleware                                      |
| `CreateCORSMiddlewareFromConfig`             | CORS middleware                                                 |
| `CreateIdempotencyMiddlewareFromConfig`      | Idempotency middleware (requires storage dependency)             |
| `CreateLimiterMiddlewareFromConfig`          | Rate limiter middleware (requires limiter dependency)            |
| `CreateLoggerMiddlewareFromConfig`           | Structured request logging middleware                           |
| `CreatePrometheusMiddlewareFromConfig`       | Prometheus metrics middleware                                   |
| `CreateRealIPMiddlewareFromConfig`           | Real client IP extraction middleware                            |
| `CreateRecoveryMiddlewareFromConfig`         | Panic recovery middleware                                       |
| `CreateRequestIDMiddlewareFromConfig`        | Request ID extraction/generation middleware                     |
| `CreateSecurityHeadersMiddlewareFromConfig`  | Security headers middleware                                     |
| `CreateTracingMiddlewareFromConfig`          | Distributed tracing middleware (requires tracer dependency)      |
