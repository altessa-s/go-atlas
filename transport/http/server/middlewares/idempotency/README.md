# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
```

Package `idempotency` provides middleware for idempotent HTTP request handling. Prevents duplicate processing of non-safe methods (POST,
PUT, PATCH, DELETE) via the Idempotency-Key header (lowercase UUID v4). The middleware acquires a lock in the configured storage before
the handler runs; on success the key is marked complete, on failure it is deleted so the client can retry. Safe methods (GET, HEAD,
OPTIONS) bypass the check. Supports configurable fallback behavior when storage is unavailable.

## Security

Keys are caller-chosen and may embed user data, and the invalid-format branch sees values that failed validation. `WithKeyLogMode`
controls how the key reaches debug-level logs.

| Mode           | Value    | Logged attribute                               |
|----------------|----------|------------------------------------------------|
| `KeyLogHashed` | `hashed` | `key_hash` — 16 hex chars of SHA-256 (default) |
| `KeyLogFull`   | `full`   | `key` — the raw key                            |
| `KeyLogOff`    | `off`    | none                                           |

Hashing is log hygiene rather than a privacy guarantee — a key drawn from a small or guessable set can still be recovered by hashing
candidates. For end-to-end redaction, run production loggers at Info or above, or wrap the handler with
[`observability/slog/handler/masking`](../../../../../observability/slog/handler/masking/README.md).
