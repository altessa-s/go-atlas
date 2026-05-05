# client

```go
import "github.com/altessa-s/go-atlas/transport/http/client"
```

Package `client` provides a resilient HTTP client with automatic retries, circuit breaker protection, rate limiting, and SSRF safeguards.
Two constructors are available: `New` returns a bare `*http.Client` suitable for any code expecting the standard type, and `NewHTTPClient`
returns an `HTTPClient` with convenience methods (Get, PostJSON, fluent RequestBuilder) layered on top.

## Features

| Feature            | Description                                                                                                            |
|--------------------|------------------------------------------------------------------------------------------------------------------------|
| Connection pooling | Automatic TCP connection pooling and keep-alive configuration via pooled transport                                     |
| Retry              | Exponential backoff with jitter, configurable min/max wait and attempt count                                           |
| Circuit breaker    | Per-host circuit breakers with configurable thresholds and state-change callbacks                                      |
| Rate limiting      | Pluggable client-side rate limiting via `WithLimiter` and `limiters.RequestsLimiter`                                   |
| SSRF protection    | Blocks connections to private/local IPs after DNS resolution, with CIDR allowlist                                      |
| Proxy              | Declarative HTTP/SOCKS5 proxy via `WithProxy`/`WithProxyURL`/`WithProxyFunc`, or `WithoutProxy` to bypass env defaults |
| Structured errors  | Typed errors with `errors.Is` matching and extraction helpers for each failure mode                                    |

## Options

| Option                       | Default       | Description                                                                                                                                          |
|------------------------------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `WithRetryMax`               | 3             | Maximum number of retry attempts                                                                                                                     |
| `WithRetryWait`              | 500ms/10s     | Minimum and maximum wait between retries (exponential backoff)                                                                                       |
| `WithBreakerMaxRequests`     | 1             | Requests allowed in half-open state                                                                                                                  |
| `WithBreakerInterval`        | 60s           | Period to clear failure counts in closed state                                                                                                       |
| `WithBreakerTimeout`         | 60s           | Duration before transitioning from open to half-open                                                                                                 |
| `WithBreakerName`            | auto          | Circuit breaker name for identification                                                                                                              |
| `WithCircuitBreakerSettings` | --            | Per-host circuit breaker configuration                                                                                                               |
| `WithMaxResponseSize`        | 0             | Maximum response body size in bytes (0 = unlimited)                                                                                                  |
| `WithLimiter`                | nil           | Pluggable client-side rate limiter                                                                                                                   |
| `WithSSRFProtection`         | false         | Enable blocking of connections to private/local IP addresses                                                                                         |
| `WithSSRFAllowedCIDRs`       | --            | CIDR prefixes exempted from SSRF blocking                                                                                                            |
| `WithLogger`                 | nil           | Structured logger for request logging                                                                                                                |
| `WithClient`                 | pooled        | Custom `*http.Client` base                                                                                                                           |
| `WithTransport`              | --            | Custom `*http.Transport` override                                                                                                                    |
| `WithProxy`                  | env           | Route requests through `http://host:port` with optional `*url.Userinfo`                                                                              |
| `WithProxyURL`               | env           | Route requests through any proxy URL (http, https, socks5, socks5h)                                                                                  |
| `WithProxyFunc`              | env           | Custom resolver matching `http.Transport.Proxy` signature                                                                                            |
| `WithoutProxy`               | env           | Disable proxy resolution, including `HTTP_PROXY`/`HTTPS_PROXY` defaults                                                                              |
| `WithProxyTLSConfig`         | system        | Custom `*tls.Config` for the handshake to an `https://` proxy (self-signed CA, mTLS); isolates proxy TLS from destination TLS via custom DialContext |
| `WithErrorHandler`           | nil           | Custom error handler                                                                                                                                 |
| `WithRetryPolicyHandler`     | nil           | Custom retry policy handler                                                                                                                          |
| `WithMetricsSubsystem`       | `http_client` | Prometheus subsystem for emitted metrics; override per upstream so multiple clients can share one registry                                           |
| `WithHealthCoordinator`      | nil           | Opts the client into `observability/health`; aggregate status derives from breaker state and retry rate                                              |
| `WithHealthServiceName`      | `http_client` | Service name used for the aggregate health checker; mirrors `WithMetricsSubsystem` for visual consistency                                            |
| `WithPerHostHealthChecks`    | off           | Also register `<service>.<host>` checkers for hosts configured via `WithCircuitBreakerSettings`                                                      |
| `WithHealthRetryWindow`      | 60s           | Rolling window for retry-rate contribution to health status                                                                                          |
| `WithHealthRetryThreshold`   | 0.20          | Retry-rate fraction at or above which the aggregate reports `Degraded`                                                                               |
| `WithHealthRetryMinSamples`  | 10            | Minimum requests in the window before retry rate is allowed to lower status                                                                          |

## Errors

| Error type              | Sentinel                  | Description                                         |
|-------------------------|---------------------------|-----------------------------------------------------|
| `UnexpectedStatusError` | `ErrUnexpectedStatus`     | HTTP response with unexpected 4xx/5xx status code   |
| `CircuitBreakerError`   | `ErrCircuitBreakerOpen`   | Request rejected because circuit breaker is open    |
| `ResponseSizeError`     | `ErrResponseSizeExceeded` | Response Content-Length exceeds configured limit    |
| `RateLimitError`        | `ErrRateLimited`          | Request denied by client-side rate limiter          |
| `RetryExhaustedError`   | `ErrMaxRetriesExceeded`   | All retry attempts exhausted, wraps last error      |
| `SSRFError`             | `ErrSSRFBlocked`          | Connection to private/local address blocked         |
| `NonRetryableError`     | `ErrNonRetryable`         | Error that must not be retried (context, TLS, SSRF) |

## Interfaces

| Interface          | Description                                                                                                                                                                                                          |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `HTTPClientSetter` | Optional `SetHTTPClient(*http.Client)` hook a sub-component implements so a parent can hand it the same shared client (proxy, retry, breaker apply once). Used by `auth/oidc.Provider` to wire `URLRevocationLoader` |

## Health

Pass `WithHealthCoordinator(coord)` to register the client with
`observability/health`. The aggregate checker (default service name
`http_client`) reports:

| Status        | When                                                                              |
|---------------|-----------------------------------------------------------------------------------|
| `Serving`     | every breaker is closed AND retry rate ≤ threshold (or sample below `MinSamples`) |
| `Degraded`    | any half-open, ≥ 1 open while ≥ 1 closed exists, OR retry rate above threshold    |
| `NotServing`  | every breaker is open (and at least one breaker exists)                           |

Coordinator subscribers receive an immediate push on every breaker state
transition (the recompute runs in a goroutine because the gobreaker callback
holds the breaker's internal mutex). Pull-based callers (`/healthz`,
`/readyz`, gRPC `Check`) read the cached value through `CheckStatus`.

Add `WithPerHostHealthChecks()` to also register `http_client.<host>` for
each host configured via `WithCircuitBreakerSettings`. Lazy hosts (no
explicit settings) only contribute to the aggregate. Tune retry-rate input
via `WithHealthRetryWindow`, `WithHealthRetryThreshold`, and
`WithHealthRetryMinSamples`.

```go
coord := health.New()
defer coord.Close()

c := client.New(
    client.WithHealthCoordinator(coord),
    client.WithCircuitBreakerSettings("api.example.com", &client.CircuitBreakerSettings{Name: "api"}),
    client.WithPerHostHealthChecks(),
)

// Whatever already serves /healthz from the same coordinator now reflects
// the HTTP client's circuit breaker and retry-rate state.
_ = c
```

## Subpackages

| Package                    | Description                                               |
|----------------------------|-----------------------------------------------------------|
| [limiters](./limiters)     | Pluggable rate limiting layer for HTTP round-trippers     |
