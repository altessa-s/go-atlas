# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
```

Package `idempotency` provides middleware for idempotent HTTP request handling. Prevents duplicate processing of non-safe methods (POST,
PUT, PATCH, DELETE) via the Idempotency-Key header (lowercase UUID v4). The middleware acquires a lock in the configured storage before
the handler runs; on success the key is marked complete, on failure it is deleted so the client can retry. Safe methods (GET, HEAD,
OPTIONS) bypass the check. Supports configurable fallback behavior when storage is unavailable.

## Security

Debug-level logs include the client-supplied idempotency key verbatim — including, in the invalid-format branch, raw values that failed
validation. Keys are caller-chosen and may embed user data. Run production loggers at Info or above, or wrap the handler with
[`observability/slog/handler/masking`](../../../../../observability/slog/handler/masking/README.md).
