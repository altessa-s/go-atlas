# limiters

```go
import "github.com/altessa-s/go-atlas/transport/http/client/limiters"
```

Package `limiters` provides a pluggable rate limiting layer for HTTP clients. Implement the `RequestsLimiter` interface with any rate limiting strategy
(token bucket, sliding window, etc.) and wrap an existing `http.RoundTripper` with `NewRoundTripper` to enforce per-host request limits at the transport
level. The resulting RoundTripper is safe for concurrent use.

A denial is returned as a `*RateLimitError` for the request host that wraps the limiter's error (`errors.Is(err, ErrRateLimited)`, the limiter's
sentinel or context error stays visible); a limiter that already returns a `*RateLimitError` (for example with `RetryAfter`) is passed through. The
error never quotes the request URL. `client.WithLimiter` installs this RoundTripper, so both paths classify the same.

## Key types

| Type / Interface    | Description                                                                            |
|---------------------|----------------------------------------------------------------------------------------|
| `RequestsLimiter`   | Interface: `Allow(ctx, key) error` — checks if a request for a given key is allowed    |
| `RoundTripper`      | Rate-limiting `http.RoundTripper` that delegates to a `RequestsLimiter` per host       |
| `RateLimitError`    | Denial of a request; matches `ErrRateLimited`, wraps the limiter's error               |
| `Func`              | Adapter: a plain `func(ctx, key) error` as a `RequestsLimiter`                         |

## Functions

| Function          | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| `NewRoundTripper` | Wraps an `http.RoundTripper` with per-host rate limiting via `RequestsLimiter`           |
| `IsRateLimitError`| Extracts a `*RateLimitError` from an error chain (nil otherwise)                         |
| `Chain`           | Consults limiters in order and stops at the first refusal; skips nil, returns nil or the single limiter as is |

## Subpackages

| Package                    | Description                                                                 |
|----------------------------|-----------------------------------------------------------------------------|
| [hostpacer](./hostpacer)   | Waiting per-host token-bucket limiter (in memory, lossless eviction)        |
