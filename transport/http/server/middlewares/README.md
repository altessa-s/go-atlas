# middlewares

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares"
```

Package `middlewares` provides HTTP middleware interfaces, utilities, and dependency-based ordering. All chainable middlewares implement
the `Middleware` interface which provides `Name()` for identification and `Handler()` for wrapping an `http.Handler`. `BaseMiddleware`
is an embeddable struct providing path filtering, logging helpers, and response-writer wrapping. `Chain` collects middlewares and applies
them to a final handler with topological ordering and deduplication. `ConditionalMiddleware` enables runtime toggling.

## Key types

| Type / Interface        | Description                                                                    |
|-------------------------|--------------------------------------------------------------------------------|
| `Middleware`            | Interface: `Name() string` + `Handler(http.Handler) http.Handler`              |
| `BaseMiddleware`        | Embeddable struct with path filtering, logging, and response-writer wrapping   |
| `Chain`                 | Middleware collection with dependency ordering, deduplication, and clone/extend |
| `ConditionalMiddleware` | Wraps a middleware with a runtime toggle for conditional activation             |
| `MiddlewareError`       | Carries HTTP status code, user-facing message, and machine-readable error code |

## Subpackages

| Package                                    | Description                                                    |
|--------------------------------------------|----------------------------------------------------------------|
| [audit](./audit)                           | Automatic request auditing via `data/audit.Auditor`            |
| [bodylimit](./bodylimit)                   | Request body size enforcement (Content-Length + streaming)      |
| [cors](./cors)                             | Cross-Origin Resource Sharing header management                |
| [defaults](./defaults)                     | Shared default configurations and constants                    |
| [driver](./driver)                         | Driven Middleware pattern with pre/post request hooks           |
| [idempotency](./idempotency)              | Idempotent request handling via Idempotency-Key header         |
| [limiter](./limiter)                       | Per-request rate limiting with standard headers                |
| [logger](./logger)                         | Structured request/response logging                            |
| [prometheus](./prometheus)                 | Prometheus metrics collection for HTTP requests                |
| [realip](./realip)                         | Real client IP extraction from request headers                 |
| [recovery](./recovery)                     | Panic recovery with stack trace logging                        |
| [requestid](./requestid)                   | UUID v4 request ID extraction or generation                    |
| [securityheaders](./securityheaders)       | Security-related HTTP response headers                         |
| [tracing](./tracing)                       | Distributed tracing with W3C Trace Context propagation         |
