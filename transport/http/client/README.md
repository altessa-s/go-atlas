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
| `WithRetryFactor`            | 1.5           | Exponential growth of the retry backoff per attempt                                                                                                  |
| `WithRetryJitter`            | 0.25          | Fraction (0–1) of each backoff added as random jitter; 0 = deterministic                                                                             |
| `WithRetryMaxElapsed`        | 0             | Wall-clock bound of all attempts of one request; a retry that would end past it is not started (0 = none)                                           |
| `WithAttemptTimeout`         | 0             | Bound each attempt (limiter wait under `WithLimiterPerAttempt`, connection, reply, body) instead of the whole request; retries and Retry-After sleeps are not cut short |
| `WithRetryAfterMax`          | 1m            | Cap on an honored `Retry-After` of a 429/503 reply: the retry waits at least that long, beyond `WithRetryWaitMax`; 0 ignores `Retry-After`            |
| `WithBreakerMaxRequests`     | 3             | Requests allowed in half-open state                                                                                                                  |
| `WithBreakerInterval`        | 30s           | Period to clear failure counts in closed state                                                                                                       |
| `WithBreakerTimeout`         | 30s           | Duration before transitioning from open to half-open                                                                                                 |
| `WithBreakerName`            | auto          | Circuit breaker name for identification                                                                                                              |
| `WithCircuitBreakerSettings` | --            | Per-host circuit breaker configuration                                                                                                               |
| `WithMaxHostBreakers`        | 1024          | Cap on breakers created lazily for hosts without explicit settings; further hosts share one breaker (0 = all such hosts share it)                   |
| `WithMaxResponseSize`        | 0             | Maximum response body size in bytes (0 = unlimited)                                                                                                  |
| `WithLimiter`                | nil           | Pluggable client-side rate limiter                                                                                                                   |
| `WithLimiterPerAttempt`      | off           | Place the limiter under the retry layer so every retry attempt passes it; a refusal is never retried                                               |
| `WithSSRFProtection`         | true          | Enable blocking of connections to private/local IP addresses                                                                                         |
| `WithoutSSRFProtection`      | --            | Disable SSRF protection; required for explicit proxies, custom TLS dialers, and non-`*http.Transport` round trippers                                 |
| `WithSSRFAllowedCIDRs`       | --            | CIDR prefixes exempted from SSRF blocking                                                                                                            |
| `WithLogger`                 | discard       | Structured logger for request logging                                                                                                                |
| `WithClient`                 | pooled        | Custom `*http.Client` base                                                                                                                           |
| `WithCookieJar`              | nil           | Cookie jar owned by the retry layer (per attempt and per redirect hop, each reply stored once); a `WithClient` jar is adopted the same way |
| `WithTransport`              | --            | Custom `*http.Transport` override                                                                                                                    |
| `WithProxy`                  | none          | Route requests through `http://host:port` with optional `*url.Userinfo`                                                                              |
| `WithProxyURL`               | none          | Route requests through any proxy URL (http, https, socks5, socks5h)                                                                                  |
| `WithProxyFunc`              | none          | Custom resolver matching `http.Transport.Proxy` signature                                                                                            |
| `WithoutProxy`               | none          | Disable proxy resolution, including `HTTP_PROXY`/`HTTPS_PROXY` defaults                                                                              |
| `WithProxyTLSConfig`         | system        | Custom `*tls.Config` for the handshake to an `https://` proxy (self-signed CA, mTLS); isolates proxy TLS from destination TLS via custom DialContext |
| `WithErrorHandler`           | nil           | Custom error handler                                                                                                                                 |
| `WithRetryPolicyHandler`     | nil           | Custom retry policy handler                                                                                                                          |
| `WithoutRetry`               | --            | One attempt per request; the response for any status (body unread) or the raw transport error is returned; overrides `WithRetryMax` and `WithRetryPolicyHandler` |
| `WithMetricsSubsystem`       | `http_client` | Prometheus subsystem for emitted metrics; override per upstream so multiple clients can share one registry                                           |
| `WithCollector`              | no-op         | Metrics collector used to emit client metrics                                                                                                        |
| `WithHealthCoordinator`      | nil           | Opts the client into `observability/health`; aggregate status derives from breaker state and retry rate                                              |
| `WithHealthServiceName`      | `http_client` | Service name used for the aggregate health checker; mirrors `WithMetricsSubsystem` for visual consistency                                            |
| `WithPerHostHealthChecks`    | off           | Also register `<service>.<host>` checkers for hosts configured via `WithCircuitBreakerSettings`                                                      |
| `WithHealthRetryWindow`      | 60s           | Rolling window for retry-rate contribution to health status                                                                                          |
| `WithHealthRetryBuckets`     | 6             | Number of buckets in the retry-rate ring                                                                                                             |
| `WithHealthRetryThreshold`   | 0.20          | Retry-rate fraction above which the aggregate reports `Degraded`                                                                                     |
| `WithHealthRetryMinSamples`  | 10            | Minimum requests in the window before retry rate is allowed to lower status                                                                          |

With SSRF protection on (the default) environment proxies are ignored (`HTTP_PROXY`/`HTTPS_PROXY` apply only after `WithoutSSRFProtection()`);
explicit proxy options are allowed and guarded, see [SSRF and proxy policy](#ssrf-and-proxy-policy).

## Cookies

With a cookie jar — `WithCookieJar(jar)`, or the `Jar` of a `WithClient` client — the retry layer owns cookie handling and the returned
`*http.Client` has no `Jar`. Every attempt and every redirect hop sends the caller's own `Cookie` header plus the jar's cookies for its URL, read at
attempt time, and every reply's cookies are stored exactly once, when it is received — retried, failed and redirect replies included (a 503 challenge
cookie reaches the retry and the next request). A cookie another request changed, deleted or let expire meanwhile is sent in its current state, and a
retry cut short never re-stores an old reply over a newer value. The rest mirrors `http.Client`'s own jar handling: cookies are looked up and stored
under `req.Host` when it is set (a virtual host), and on a redirect chain an initial cookie that a redirect reply replaced or deleted is no longer
forwarded (the caller's other cookies are, rebuilt sorted). That filtering runs in the client's redirect policy before the caller's `CheckRedirect`
(or net/http's default, 10 redirects), so cookies the policy sets on the next request are kept.

## Retry-After

The retry layer honors `Retry-After` on 429 and 503 replies (delta seconds or an HTTP date): the next retry waits at least as long as the server asked,
capped by `WithRetryAfterMax` (default `DefaultRetryAfterMax`, one minute), and never shorter than the exponential backoff. `WithRetryAfterStatuses`
replaces the honored statuses (default `DefaultRetryAfterStatuses()`: 429, 503), e.g. for a retry policy that also retries 500/502/504 replies.
`ParseRetryAfter(h, now, max)` exposes the same parser for callers that run their own retry loop; it reports `ok == false` for an absent or malformed
header.

## Errors

| Error type              | Sentinel                  | Description                                         |
|-------------------------|---------------------------|-----------------------------------------------------|
| `UnexpectedStatusError` | `ErrUnexpectedStatus`     | HTTP response with unexpected 4xx/5xx status code   |
| `CircuitBreakerError`   | `ErrCircuitBreakerOpen`   | Request rejected because circuit breaker is open    |
| `ResponseSizeError`     | `ErrResponseSizeExceeded` | Response Content-Length exceeds configured limit    |
| `RetryExhaustedError`   | `ErrMaxRetriesExceeded`   | All retry attempts exhausted, wraps last error (see below) |
| `SSRFError`             | `ErrSSRFBlocked`          | Connection to private/local address blocked         |
| `NonRetryableError`     | `ErrNonRetryable`         | Error that must not be retried (context, TLS, SSRF) |

A request whose retry loop stops on a retryable failure because no further attempt is allowed — the retries, the `WithRetryMaxElapsed` budget or
the delay policy ran out, the context still alive — fails with `RetryExhaustedError` (`Attempts`, `Method`, `URL` without query string or user
info). It wraps the last transport error, or, for a retryable status, the last reply's `UnexpectedStatusError`, so `IsUnexpectedStatusError` reports
the status either way. A failure that is not retried keeps its own error, and a context that ends while waiting for a retry returns the context
error. A `WithErrorHandler` handler receives the `RetryExhaustedError` as its error.

By default the limiter wraps the whole retry loop, so only the first attempt passes it; `WithLimiterPerAttempt()` moves it under the retry layer,
so every attempt waits on a pacing limiter (see [`limiters/hostpacer`](limiters/hostpacer/README.md)) or is refused by a budget — a refusal ends the
request without a retry. With `WithAttemptTimeout` that wait shares the attempt's deadline. A request denied by the `WithLimiter` limiter fails with
[`limiters.RateLimitError`](limiters/README.md)
(`errors.Is(err, limiters.ErrRateLimited)`), which wraps the limiter's error.

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

Protected clients disable environment proxy lookup. An explicit proxy (`WithProxy`, `WithProxyURL`, `WithProxyFunc`) is allowed: the connection to the
proxy goes through the guarded dialer, so a proxy on a private address needs its network in `WithSSRFAllowedCIDRs`; the proxy resolves and dials the
target host, so a private target named by host name is not blocked by the client — enforce egress restrictions at the proxy. A target given as a literal
IP address in a blocked range is refused with an `SSRFError` before it reaches the proxy. A proxy TLS configuration (`WithProxyTLSConfig`), custom TLS
dialers, a proxy resolver set on a caller transport and custom round trippers cannot enforce the policy; `New` and `NewHTTPClient` panic with an error
wrapping `ErrSSRFUnsafeTransport` when given these configurations. Use `WithoutSSRFProtection()` for trusted proxy/custom-transport clients and enforce
destination restrictions in that transport or proxy. Caller-owned clients and transports are cloned before policy changes.

`SSRFDialControl(allowed...)` exports the dial guard (a `net.Dialer.Control` hook) for services that build their own transport: it rejects private,
loopback, link-local and reserved resolved addresses with an `SSRFError`, checks IPv4-mapped addresses as IPv4 and ignores zones.