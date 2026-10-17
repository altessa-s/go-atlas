# middlewares

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares"
```

Package `middlewares` provides HTTP middleware interfaces, utilities, and dependency-based ordering. All chainable middlewares implement the
`Middleware` interface which provides `Name()` for identification and `Handler()` for wrapping an `http.Handler`. `BaseMiddleware` is an embeddable
struct providing path filtering, logging helpers, and response-writer wrapping. `Chain` collects middlewares and `Then` applies them to a final handler
in insertion order; `Ordered` returns a dependency-sorted copy to apply instead (rejecting duplicate names) and `OrderedWithDedupe` drops duplicates
before sorting. `ConditionalMiddleware` applies a middleware only to requests accepted by a `Matcher`.

## Key types

| Type / Interface        | Description                                                                    |
|-------------------------|--------------------------------------------------------------------------------|
| `Middleware`            | Interface: `Name() string` + `Handler(http.Handler) http.Handler`              |
| `BaseMiddleware`        | Embeddable struct with path filtering, logging, and response-writer wrapping   |
| `Chain`                 | Middleware collection with dependency ordering, deduplication, and clone/extend |
| `ConditionalMiddleware` | Function wrapping a middleware with a `Matcher` evaluated on every request      |

## Middleware Identity

Every chainable middleware sub-package exports two package-level identification helpers, and its middleware type declares ordering through methods:

| Export                           | Type                | Purpose                                                                              |
|----------------------------------|---------------------|--------------------------------------------------------------------------------------|
| `Name()`                         | `func() string`     | Returns the middleware name as a plain string                                        |
| `ID`                             | `Middleware`        | Lightweight typed reference for factory exclusion lists (`cors.ID`)                  |
| `Dependencies()` method          | `[]string`          | Optional predecessors, built from sibling `Name()` calls instead of string literals  |
| `RequiredDependencies()` method  | `[]string`          | Predecessors that must be present in the chain                                       |

## Subpackages

| Package                                    | Description                                                    |
|--------------------------------------------|----------------------------------------------------------------|
| [audit](./audit)                           | Automatic request auditing via `data/audit.Auditor`            |
| [auth](./auth)                             | Authentication with Bearer tokens, API keys, and custom schemes |
| [bodylimit](./bodylimit)                   | Request body size enforcement (Content-Length + streaming)      |
| [cors](./cors)                             | Cross-Origin Resource Sharing header management                |
| [defaults](./defaults)                     | Shared default configurations and constants                    |
| [driver](./driver)                         | Driven Middleware pattern with pre/post request hooks           |
| [geoacl](./geoacl)                         | Country/region-based access control on caller IP                |
| [idempotency](./idempotency)              | Idempotent request handling via Idempotency-Key header         |
| [ipacl](./ipacl)                           | IP/CIDR allow- and deny-list enforcement                        |
| [limiter](./limiter)                       | Per-request rate limiting with standard headers                |
| [logger](./logger)                         | Structured request/response logging                            |
| [metrics](./metrics)                       | Prometheus metrics collection for HTTP requests                |
| [realip](./realip)                         | Real client IP extraction from request headers                 |
| [recovery](./recovery)                     | Panic recovery with stack trace logging                        |
| [requestid](./requestid)                   | UUID v4 request ID extraction or generation                    |
| [securityheaders](./securityheaders)       | Security-related HTTP response headers                         |
| [tracing](./tracing)                       | Distributed tracing with W3C Trace Context propagation         |
