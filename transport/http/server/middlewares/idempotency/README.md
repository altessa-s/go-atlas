# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
```

Package `idempotency` provides middleware for idempotent HTTP request handling. Prevents duplicate processing of non-safe methods (POST,
PUT, PATCH, DELETE) via the Idempotency-Key header (lowercase UUID v4). The middleware acquires a lock in the configured storage before
the handler runs; on success the key is marked complete, on failure it is deleted so the client can retry. Safe methods (GET, HEAD,
OPTIONS) bypass the check. Supports configurable fallback behavior when storage is unavailable.
