# limiter

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
```

Package `limiter` provides middleware for per-request rate limiting. Delegates rate decisions to a `limiters.Limiter` backend (e.g. token
bucket). Enriches the limiter context with client IP (from the realip middleware) and Bearer token when available. Sets standard headers
(X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After) automatically. Returns 429 Too Many Requests on limit
exceeded. Declares a dependency on `realip` for ordering.
