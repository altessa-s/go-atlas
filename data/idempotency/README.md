# idempotency

```go
import "github.com/altessa-s/go-atlas/data/idempotency"
```

Package `idempotency` provides duplicate request detection using idempotency keys. Supports multiple storage backends (memory, Redis, NATS) for
single-instance and distributed systems.

## Stolen-lock detection

`AttemptLock` returns a `*State` carrying an opaque CAS token. `Complete` requires that same `*State` back; if the lock has been taken over
by another holder between `AttemptLock` and `Complete` (typically because the lock TTL fired during processing), Complete returns
`ErrLockStolen` instead of silently overwriting the new holder's result.

Backend-specific CAS primitives:

| Backend | Mechanism |
|---|---|
| memory | mutex-protected byte comparison of stored value vs lockToken |
| redis | Lua script: `GET == lockToken ? SET : ErrLockStolen` |
| nats | `kv.Update(key, val, revision)` — server rejects on revision mismatch |

Pass the `*State` from AttemptLock straight back to Complete:

```go
ok, state, err := keeper.AttemptLock(ctx, key)
if err != nil { return err }
if !ok { /* in-progress or completed; consult state */ return nil }
defer keeper.Delete(ctx, key)  // on failure
result, err := doWork(ctx)
if err != nil { return err }
return keeper.Complete(ctx, key, result, state)  // state carries the CAS token
```

Passing `nil` state to Complete returns `ErrMissingLockState` — the guard cannot be silently disabled.

## Options

| Option           | Default | Description                  |
|------------------|---------|------------------------------|
| `WithLogger`     | discard | Structured logger            |
| `WithSerializer` | JSON    | Serialization format         |

## Subpackages

| Package                                  | Description                          |
|------------------------------------------|--------------------------------------|
| [factory](./factory)                     | Configuration-based creation         |
| [storages/memory](./storages/memory)     | In-memory backend with TTL           |
| [storages/redis](./storages/redis)       | Distributed Redis storage            |
| [storages/nats](./storages/nats)         | NATS JetStream storage               |
