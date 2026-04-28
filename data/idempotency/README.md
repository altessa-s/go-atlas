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

## Per-call TTL

`AttemptLockWithTTL(ctx, key, lockTtl)` and `CompleteWithTTL(ctx, key, data, lockState, resultTtl)` accept per-call TTL overrides. Pass `0` to
fall back to the backend's configured default. Lock and result TTLs are independent — short lock with long result is the typical webhook
deduplication shape:

```go
ok, state, err := keeper.AttemptLockWithTTL(ctx, webhookID, 30*time.Second) // short lock
if err != nil { return err }
if !ok { /* in-flight or completed */ return nil }

result, err := process(ctx)
if err != nil { _ = keeper.Delete(ctx, webhookID); return err }

return keeper.CompleteWithTTL(ctx, webhookID, result, state, 24*time.Hour)  // long result
```

Backend matrix:

| Backend | AttemptLockWithTTL | CompleteWithTTL |
|---|---|---|
| memory | per-entry `expiresAt` | per-entry `expiresAt` |
| redis | `SET NX ... PX <ttl>` | Lua: `SET ... PX <ttl>` |
| nats | `kv.Create(... jetstream.KeyTTL(ttl))` (requires NATS 2.11+) | **`ErrPerCallTtlNotSupported`** for resultTtl > 0 |

The NATS backend cannot honor a per-call result TTL — `nats.go` v1.51.0 doesn't expose per-message TTL on `KV.Put`/`Update`. Callers either
pass `resultTtl=0` (falls back to bucket TTL) or migrate to a backend that supports the override.

For code paths that may run against multiple backends (e.g. NATS in production, memory in tests), capability bits report which `WithTTL`
methods the configured backend honors:

| Backend | `SupportsAttemptLockWithTTL()` | `SupportsCompleteWithTTL()` |
|---|---|---|
| memory | `true` | `true` |
| redis  | `true` | `true` |
| nats   | `true` | `false` (KV.Put has no TTL option) |

The Keeper delegates both to the underlying storage. Use the bits to choose a code path before calling the WithTTL methods:

```go
if keeper.SupportsCompleteWithTTL() {
    return keeper.CompleteWithTTL(ctx, key, result, state, 24*time.Hour)
}
return keeper.Complete(ctx, key, result, state) // bucket TTL
```

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
