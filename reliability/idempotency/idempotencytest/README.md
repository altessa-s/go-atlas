# idempotencytest

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/idempotencytest"
```

Test kit for [`reliability/idempotency`](../): it MockIdempotencyStorage, a concurrency-safe in-memory idempotency storage for tests of idempotency consumers (HTTP and gRPC middleware, broker deduplication).

## Key types

| Symbol | Description |
|--------|-------------|
| `MockIdempotencyStorage` | In-memory `storages.Storage` with CAS semantics and raw entry access (`SetEntry`, `Entry`, `Delete`) |
| `NewMockIdempotencyStorage()` | Returns an empty storage |
