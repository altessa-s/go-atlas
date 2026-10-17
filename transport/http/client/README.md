# client

```go
import "github.com/altessa-s/go-atlas/transport/http/client"
```

Package `client` provides a resilient HTTP client with automatic retries, circuit breaker protection, rate limiting, and SSRF safeguards. Two
constructors are available: `New` returns a bare `*http.Client` suitable for any code expecting the standard type, and `NewHTTPClient` returns an
`HTTPClient` with convenience methods (Get, PostJSON, fluent RequestBuilder) layered on top.

## Features

| Feature            | Description                                                                                                            |
|--------------------|------------------------------------------------------------------------------------------------------------------------|
| Connection pooling | Automatic TCP connection pooling and keep-alive configuration via pooled transport                                     |
| Retry              | Exponential backoff with jitter, configurable min/max wait and attempt count                                           |
| Circuit breaker    | A breaker per destination host (bounded by `WithMaxHostBreakers`), with per-host overrides and state-change callbacks  |
| Rate limiting      | Pluggable client-side rate limiting via `WithLimiter` and `limiters.RequestsLimiter`                                   |
| SSRF protection    | Blocks connections to private/local IPs after DNS resolution, with CIDR allowlist                                      |
| Proxy              | Declarative HTTP/SOCKS5 proxy via `WithProxy`/`WithProxyURL`/`WithProxyFunc`, or `WithoutProxy` to bypass env defaults |
| Structured errors  | Typed errors with `errors.Is` matching and extraction helpers for each failure mode                                    |

## Options

| Option                       | Default       | Description                                                                                                                                          |
|------------------------------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `WithRetryMax`               | 5             | Maximum number of retry attempts                                                                                                                     |
| `WithRetryWait`              | 500ms/10s     | Minimum and maximum wait between retries (exponential backoff)                                                                                       |
| `WithRetryWaitMin`           | 500ms         | Minimum wait between retries                                                                                                                         |
| `WithRetryWaitMax`           | 10s           | Maximum wait between retries                                                                                                                         |
| `WithBreakerMaxRequests`     | 3             | Requests allowed in half-open state                                                                                                                  |
| `WithBreakerInterval`        | 30s           | Period to clear failure counts in closed state                                                                                                       |
| `WithBreakerTimeout`         | 30s           | Duration before transitioning from open to half-open                                                                                                 |
| `WithBreakerName`            | auto          | Circuit breaker name for identification                                                                                                              |
| `WithCircuitBreakerSettings` | --            | Per-host circuit breaker configuration                                                                                                               |
| `WithMaxHostBreakers`        | 1024          | Cap on breakers created lazily for hosts without explicit settings; further hosts share one breaker (0 = all such hosts share it)                   |
| `WithMaxResponseSize`        | 0             | Maximum response body size in bytes (0 = unlimited)                                                                                                  |
| `WithLimiter`                | nil           | Pluggable client-side rate limiter                                                                                                                   |
| `WithSSRFProtection`         | true          | Enable blocking of connections to private/local IP addresses                                                                                         |
| `WithoutSSRFProtection`      | --            | Disable SSRF protection; required for explicit proxies, custom TLS dialers, and non-`*http.Transport` round trippers                                 |
| `WithSSRFAllowedCIDRs`       | --            | CIDR prefixes exempted from SSRF blocking                                                                                                            |
| `WithLogger`                 | discard       | Structured logger for request logging                                                                                                                |
| `WithClient`                 | pooled        | Custom `*http.Client` base                                                                                                                           |
| `WithTransport`              | --            | Custom `*http.Transport` override                                                                                                                    |
| `WithProxy`                  | none (SSRF)   | Route requests through `http://host:port` with optional `*url.Userinfo`                                                                              |
| `WithProxyURL`               | none (SSRF)   | Route requests through any proxy URL (http, https, socks5, socks5h)                                                                                  |
| `WithProxyFunc`              | none (SSRF)   | Custom resolver matching `http.Transport.Proxy` signature                                                                                            |
| `WithoutProxy`               | none (SSRF)   | Disable proxy resolution, including `HTTP_PROXY`/`HTTPS_PROXY` defaults                                                                              |
| `WithProxyTLSConfig`         | system        | Custom `*tls.Config` for the handshake to an `https://` proxy (self-signed CA, mTLS); isolates proxy TLS from destination TLS via custom DialContext |
| `WithErrorHandler`           | nil           | Custom error handler                                                                                                                                 |
| `WithRetryPolicyHandler`     | nil           | Custom retry policy handler                                                                                                                          |
| `WithMetricsSubsystem`       | `http_client` | Prometheus subsystem for emitted metrics; override per upstream so multiple clients can share one registry                                           |
| `WithCollector`              | no-op         | Metrics collector used to emit client metrics                                                                                                        |
| `WithHealthCoordinator`      | nil           | Opts the client into `observability/health`; aggregate status derives from breaker state and retry rate                                              |
| `WithHealthServiceName`      | `http_client` | Service name used for the aggregate health checker; mirrors `WithMetricsSubsystem` for visual consistency                                            |
| `WithPerHostHealthChecks`    | off           | Also register `<service>.<host>` checkers for hosts configured via `WithCircuitBreakerSettings`                                                      |
| `WithHealthRetryWindow`      | 60s           | Rolling window for retry-rate contribution to health status                                                                                          |
| `WithHealthRetryBuckets`     | 6             | Number of buckets in the retry-rate ring                                                                                                             |
| `WithHealthRetryThreshold`   | 0.20          | Retry-rate fraction above which the aggregate reports `Degraded`                                                                                     |
| `WithHealthRetryMinSamples`  | 10            | Minimum requests in the window before retry rate is allowed to lower status                                                                          |

With SSRF protection on (the default) the transport uses no proxy, environment proxies included; `HTTP_PROXY`/`HTTPS_PROXY` apply only after
`WithoutSSRFProtection()`.

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

Pass `WithHealthCoordinator(coord)` to register the client with `observability/health`. The aggregate checker (default service name `http_client`)
reports:

| Status        | When                                                                              |
|---------------|-----------------------------------------------------------------------------------|
| `Serving`     | every breaker is closed AND retry rate ≤ threshold (or sample below `MinSamples`) |
| `Degraded`    | any half-open, ≥ 1 open while ≥ 1 closed exists, OR retry rate above threshold    |
| `NotServing`  | every breaker is open (and at least one breaker exists)                           |

Coordinator subscribers receive an immediate push on every breaker state transition (the recompute runs in a goroutine because the gobreaker callback
holds the breaker's internal mutex). Pull-based callers (`/healthz`, `/readyz`, gRPC `Check`) read the cached value through `CheckStatus`.

Add `WithPerHostHealthChecks()` to also register `http_client.<host>` for each host configured via `WithCircuitBreakerSettings`. Lazy hosts (no explicit
settings) only contribute to the aggregate. Tune retry-rate input via `WithHealthRetryWindow`, `WithHealthRetryBuckets`, `WithHealthRetryThreshold`, and
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
| [factory](./factory)       | Maps health and SSRF configuration schemas to client options |
| [limiters](./limiters)     | Pluggable rate limiting layer for HTTP round-trippers     |

## SSRF and proxy policy

SSRF protection is enabled by default and blocks private, link-local and loopback destinations after DNS resolution. Allow local development explicitly
with `WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"))`.

Protected clients disable environment proxy lookup. Explicit proxies, custom TLS dialers and custom round trippers cannot enforce this policy; `New` and
`NewHTTPClient` panic with an error wrapping `ErrSSRFUnsafeTransport` when given these configurations. Use `WithoutSSRFProtection()` for trusted
proxy/custom-transport clients and enforce destination restrictions in that transport or proxy. CIDR exceptions do not make a proxy safe. Caller-owned
clients and transports are cloned before policy changes.
