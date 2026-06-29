# static

```go
import "github.com/altessa-s/go-atlas/auth/static"
```

Transport-neutral static-token authentication. Validates API keys and other pre-shared credentials against an in-memory store keyed by the
HMAC-SHA256 digest of each token. Used by the gRPC interceptor at `transport/grpc/interceptors/auth/static` and the HTTP middleware at
`transport/http/server/middlewares/auth/static`.

## Key types

| Type / Interface     | Description                                                           |
|----------------------|------------------------------------------------------------------------|
| `TokenStore`         | Validates a token and returns associated data                          |
| `InMemoryStore`      | Default store; keys tokens by their HMAC-SHA256 digest, never plaintext|
| `RateLimiter`        | Consumer-side interface for plugging an external rate limiter          |
| `RateLimitedStore`   | Decorator that gates `Validate` through a `RateLimiter`                |
| `Metrics`            | Validation telemetry (success/failure counter, latency, active tokens) |

## Options

| Option                 | Default                         | Description                                                          |
|------------------------|---------------------------------|-----------------------------------------------------------------------|
| `WithInitialTokens`    | empty                           | Token→data pairs seeded at construction time                          |
| `WithMetrics`          | nil (no-op)                     | `*Metrics` recording validations and active-token count               |
| `WithHMACKey`          | per-instance 32-byte random key | Stable digest key for cross-process consistency (min 16 bytes)        |

## Sentinel errors

| Error               | Returned when                                            |
|---------------------|----------------------------------------------------------|
| `ErrTokenInvalid`   | Token is non-empty but not registered with the store     |
| `ErrTokenEmpty`     | Token is the empty string                                |
| `ErrRateLimited`    | `RateLimitedStore` rejects a request via its `RateLimiter` |

Compare with `errors.Is` rather than `==` — adapters in the transport packages wrap these errors with additional context.

## Usage

### Basic validation

```go
store := static.NewInMemoryStore(
    static.WithInitialTokens(map[string]any{
        "sk_live_xxx": UserInfo{ID: "u1", Role: "admin"},
        "sk_test_xxx": UserInfo{ID: "u2", Role: "viewer"},
    }),
)

data, err := store.Validate(ctx, token)
if errors.Is(err, static.ErrTokenInvalid) {
    // reject
}
```

### Dynamic token management

```go
store := static.NewInMemoryStore()
store.AddToken("freshly-issued", UserInfo{ID: "u3"})
store.RemoveToken("compromised")
```

### With metrics

```go
m := static.NewMetrics(collector, "auth_api")
store := static.NewInMemoryStore(
    static.WithInitialTokens(tokens),
    static.WithMetrics(m),
)
```

### Stable cross-process digests

```go
// Supply a stable key when multiple processes must agree on the storage
// digest of a token (shared cache, distributed rate-limit keying, etc.).
store := static.NewInMemoryStore(static.WithHMACKey(secret))
```

### Rate limiting

`RateLimitedStore` wraps a `TokenStore` and consults a caller-supplied `RateLimiter`. The package does not ship an implementation — wire one
from `data/limiters/tokenbucket`, a Redis-backed limiter, or any other source.

The contract is **failure-only**: `Allow` is a pure check that MUST NOT consume budget by itself, and `RecordFailure` is the only path that
debits the per-key counter. Successful validations never touch the limiter. This shape exists to defeat the success-resets-counter brute-force
bypass: if a success could reset (or even just refund) the budget, an attacker holding a single valid token could interleave 1 valid + N invalid
attempts and the invalid attempts would never accumulate.

Because the decorator never refunds, the limiter MUST self-decay (token-bucket refill, sliding window, TTL'd counter) — a monotonic counter will
eventually lock legitimate clients out of their own tokens.

```go
limiter := myLimiter // implements static.RateLimiter
store := static.NewRateLimitedStore(inner, limiter, func(ctx context.Context) string {
    return extractClientIP(ctx)
})
```

## Security notes

- `InMemoryStore` keys tokens by their HMAC-SHA256 digest. The plaintext token is hashed at insertion time and discarded; only digests are
  retained in memory. Lookups are a single map probe, so timing depends on token length only, not on token position or membership.
- The default HMAC key is generated from `crypto/rand` once per `InMemoryStore`. Digests are not valid across restarts; pass `WithHMACKey` for
  cross-process stability.
- Always transport tokens over TLS and rotate them on a schedule. Brute-force protection should be applied at a higher layer via
  `RateLimitedStore` or a transport-level rate limiter.

## Performance

| Operation                            | Cost                                          |
|--------------------------------------|------------------------------------------------|
| `Validate` (hit/miss)                | One HMAC-SHA256 + one map probe                |
| `AddToken` / `RemoveToken`           | One HMAC-SHA256 + one map write under `sync.Mutex` |
| `TokenCount`                         | O(1) under `sync.RWMutex`                      |

The HMAC instance is recycled through a `sync.Pool`, so steady-state lookups allocate only the digest byte slice.
