# limiter

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
```

Package `limiter` provides middleware for per-request rate limiting. Delegates rate decisions to a `limiters.Limiter` backend (e.g. token bucket).
Enriches the limiter context with client IP (from the realip middleware) and Bearer token when available. Sets standard headers (X-RateLimit-Limit,
X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After) automatically. Returns 429 Too Many Requests on limit exceeded. Requires `realip` and runs after
`auth` and `mtls` when present, giving the limiter access to the authenticated principal.
