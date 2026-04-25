# limiters

```go
import "github.com/altessa-s/go-atlas/transport/http/client/limiters"
```

Package `limiters` provides a pluggable rate limiting layer for HTTP clients. Implement the `RequestsLimiter` interface with any rate limiting
strategy (token bucket, sliding window, etc.) and wrap an existing `http.RoundTripper` with `NewRoundTripper` to enforce per-host request limits
at the transport level. The resulting RoundTripper is safe for concurrent use.

## Key types

| Type / Interface    | Description                                                                            |
|---------------------|----------------------------------------------------------------------------------------|
| `RequestsLimiter`   | Interface: `Allow(ctx, key) error` — checks if a request for a given key is allowed    |
| `RoundTripper`      | Rate-limiting `http.RoundTripper` that delegates to a `RequestsLimiter` per host       |

## Functions

| Function          | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| `NewRoundTripper` | Wraps an `http.RoundTripper` with per-host rate limiting via `RequestsLimiter`           |
