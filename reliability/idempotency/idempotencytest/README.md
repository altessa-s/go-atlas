# idempotencytest

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/idempotencytest"
```

`MockIdempotencyStorage` is a concurrency-safe in-memory store for tests of [idempotency](../) consumers. It tracks lock age for takeover.
Use a real backend to exercise TTL expiry.

## Key types

| Symbol | Description |
|--------|-------------|
| `MockIdempotencyStorage` | In-memory `storages.Storage` with CAS semantics and raw entry access (`SetEntry`, `Entry`, `Delete`) |
| `NewMockIdempotencyStorage()` | Returns an empty storage |
