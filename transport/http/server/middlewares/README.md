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

| Type / Interface        | Description                                                                     |
|-------------------------|---------------------------------------------------------------------------------|
| `Middleware`            | Interface: `Name() string` + `Handler(http.Handler) http.Handler`               |
| `OutermostMiddleware`   | Runs before ordinary middleware in dependency-ordered chains                    |
| `BaseMiddleware`        | Embeddable struct with path filtering, logging, and response-writer wrapping    |
| `Chain`                 | Middleware collection with dependency ordering, deduplication, and clone/extend |
| `ConditionalMiddleware` | Function wrapping a middleware with a `Matcher` evaluated on every request      |

Outermost middleware can depend on other outermost members. A dependency on an ordinary member creates a cycle and the chain returns an error.
Conditional wrappers preserve an outermost member's placement and dependencies.

## Ignore lists and request paths

`BaseMiddleware.ShouldIgnoreRequest` backs the `WithIgnorePaths` / `WithIgnorePatterns` options of every middleware. It consults the list for a
canonical request path only, one that names the same endpoint to the policy and to the router:

- the decoded path (`r.URL.Path`) has a leading slash and no empty, `.` or `..` segment; a trailing slash is significant;
- the escaped path has no encoded slash (`%2F`, `%2f`).

Anything else (`//healthz`, `/healthz/../admin`, `/public/%2E%2E/admin`, `/v1%2Fhealthz`) is never ignored, so the middleware runs: an ignore entry
or an unanchored ignore pattern cannot be borrowed to skip authentication, access control or rate limiting for another path. The same rule makes
`ipacl` and `geoacl` answer 403 and the factory's idempotency classifier require a key.

The second condition exists because policies read the decoded path while a router may match the escaped one:

| Request            | `net/http.ServeMux` (`router/std`)                         |
|--------------------|------------------------------------------------------------|
| `/v1%2Fhealthz`    | serves `/{tenant}` with the single segment `v1/healthz`    |
| `/users/a%2Fb`     | serves `/users/{id}`                                       |
| `/a/%2E%2E/admin`  | 404; `/users/%2E%2E` is served by `/users/{id}`            |
| `//admin`, `/a/..` | 301 to the cleaned path, after the middlewares ran         |
| `/users/a%5Cb`     | serves `/users/{id}`: a backslash is an ordinary character |

Without an encoded slash the escaped and the decoded path split into the same segments, so every router dispatches on the path the policy saw. A
`RawPath` that does not encode `Path` is ignored by the routers and by the rule alike, except that an encoded slash in it still refuses the request.
`ShouldIgnoreRequest` is the only way to consult the list; there is no lookup by bare path, which would not tell how the path was escaped.

## Middleware Identity

Every chainable middleware sub-package exports two package-level identification helpers, and its middleware type declares ordering through methods:

| Export                          | Type            | Purpose                                                                             |
|---------------------------------|-----------------|-------------------------------------------------------------------------------------|
| `Name()`                        | `func() string` | Returns the middleware name as a plain string                                       |
| `ID`                            | `Middleware`    | Lightweight typed reference for factory exclusion lists (`cors.ID`)                 |
| `Dependencies()` method         | `[]string`      | Optional predecessors, built from sibling `Name()` calls instead of string literals |
| `RequiredDependencies()` method | `[]string`      | Predecessors that must be present in the chain                                      |

## Subpackages

| Package                              | Description                                                     |
|--------------------------------------|-----------------------------------------------------------------|
| [audit](./audit)                     | Automatic request auditing via the `EventEmitter` contract      |
| [auth](./auth)                       | Authentication with Bearer tokens, API keys, and custom schemes |
| [bodylimit](./bodylimit)             | Request body size enforcement (Content-Length + streaming)      |
| [cors](./cors)                       | Cross-Origin Resource Sharing header management                 |
| [defaults](./defaults)               | Shared default configurations and constants                     |
| [driver](./driver)                   | Driven Middleware pattern with pre/post request hooks           |
| [geoacl](./geoacl)                   | Country/region-based access control on caller IP                |
| [idempotency](./idempotency)         | Idempotent request handling via Idempotency-Key header          |
| [ipacl](./ipacl)                     | IP/CIDR allow- and deny-list enforcement                        |
| [limiter](./limiter)                 | Per-request rate limiting with standard headers                 |
| [logger](./logger)                   | Structured request/response logging                             |
| [metrics](./metrics)                 | Prometheus metrics collection for HTTP requests                 |
| [realip](./realip)                   | Real client IP extraction from request headers                  |
| [recovery](./recovery)               | Panic recovery with stack trace logging                         |
| [requestid](./requestid)             | UUID v4 request ID extraction or generation                     |
| [securityheaders](./securityheaders) | Security-related HTTP response headers                          |
| [tracing](./tracing)                 | Distributed tracing with W3C Trace Context propagation          |

## Response capture

`NewResponseWriter` provides the shared response recorder used by metrics and driven middleware. Call `Release` after use. It forwards informational
headers, records the final status, and counts bytes accepted by the underlying writer, including partial writes. `HeadersWritten` reports a committed
final response; `WriteError` exposes the first body write failure. Failed or truncated captures must not be replayed as complete responses.
