# httpconfig

```go
import httpconfig "github.com/altessa-s/go-atlas/config/http"
```

Package `httpconfig` defines the HTTP server schema, its middlewares, pprof, TLS and outbound SSRF protection. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                             | Description                                                                                               |
|----------------------------------|-----------------------------------------------------------------------------------------------------------|
| `Config`                           | Represents the configuration for HTTP server settings.                                                    |
| `ClientSSRF`                 | Is the YAML-driven SSRF (Server-Side Request Forgery) egress policy for transport/http/client.            |
| `ACLMiddleware`             | Is a generic base for HTTP ACL middleware configurations.                                                 |
| `MiddlewareFilter`     | Defines common path filtering settings used by HTTP middlewares.                                          |
| `BaseMiddleware`       | Provides common configuration structure for HTTP middlewares.                                             |
| `BodyLimitMiddleware`       | Defines the configuration for HTTP body size limiting middleware.                                         |
| `CORSMiddleware`            | Defines the configuration for HTTP Cross-Origin Resource Sharing (CORS) middleware.                       |
| `GeoACLMiddleware`          | Defines the configuration for the HTTP geographic access control middleware.                              |
| `IdempotencyKeyLogMode`          | Controls how the client-supplied idempotency key is rendered in the middleware's debug-level log records. |
| `IdempotencyMiddleware`     | Defines the configuration for HTTP idempotency middleware.                                                |
| `IPACLMiddleware`           | Defines the configuration for the HTTP IP access control middleware.                                      |
| `LimiterMiddleware`         | Defines the configuration for HTTP rate limiting middleware.                                              |
| `LoggerMiddleware`          | Defines the configuration for HTTP request logging middleware.                                            |
| `MetricsMiddleware`         | Defines the configuration for HTTP metrics collection middleware.                                         |
| `RealIPMiddleware`          | Defines the configuration for HTTP real IP extraction middleware.                                         |
| `RecoveryMiddleware`        | Defines the configuration for HTTP panic recovery middleware.                                             |
| `RequestIDMiddleware`       | Defines the configuration for HTTP request ID middleware.                                                 |
| `SecurityHeadersMiddleware` | Defines the configuration for HTTP security headers middleware.                                           |
| `TracingMiddleware`         | Defines the configuration for HTTP tracing middleware.                                                    |
| `Middlewares`              | Defines the configuration for HTTP middlewares.                                                           |
| `Pprof`                      | Configures pprof profiling endpoints on the HTTP server.                                                  |
| `TLS`                        | Configures Tls settings specifically for Http servers.                                                    |
| `STS`                            | Configures Http Strict Transport Security (HSTS) headers.                                                 |

## Real IP trust

`middlewares.realIP.trustedProxies` explicitly identifies peers allowed to supply forwarding headers. Private addresses alone do not grant trust.
`trustPrivatePeers: true` restores the previous implicit trust of private, loopback and reserved peers; the default is `false`.

See the [config index](../README.md) for the other schema packages.

## Request body limit

`maxBodySize` controls the factory's `Writer.Read` cap: positive values select a byte limit, negative values disable it, and zero uses 4 MiB or the
positive size of an installed bodyLimit middleware. Middleware limits still apply independently; reading `r.Body` directly bypasses the writer cap. See
[writer](../../transport/http/server/writer).
