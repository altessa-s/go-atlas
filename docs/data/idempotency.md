# Idempotency keeper (`idempotency`)

```go
import "github.com/altessa-s/go-atlas/data/idempotency"
```

A duplicate-request detector with a state machine. `AttemptLock` either claims a key (caller does the work) or returns the current `*State` of
whoever is already holding it (caller short-circuits with cached result or "in progress" signal). `Complete` writes the success state under a
CAS guard — a stale holder cannot overwrite a fresh one. Three pluggable backends: in-memory (real, with TTL), Redis, NATS JetStream KV.

---

## When to reach for `idempotency`

| Scenario | Use |
|---|---|
| HTTP / gRPC handler returning a cached response for a repeated request (`Idempotency-Key` header) | `AttemptLock` + `Complete(data)` |
| Long-running operation; concurrent caller should see "in progress, retry later" | `AttemptLock` + check `State.Status == StatusInProgress` |
| Need to know success vs failure from a previous run | `state.Status` enum + `state.Data` payload |
| Operation that may exceed lock TTL — protect against stolen-lock overwrite | `Complete` returns `ErrLockStolen` |
| Different lock vs result lifetimes (short lock, long result cache) | `AttemptLockWithTTL` + `CompleteWithTTL` |
| Failure cleanup so the next attempt can re-claim | `Delete` |

---

## Quick start

### Memory (tests, single-process)

```go
storage := memorystorage.New(memorystorage.WithTtl(time.Hour))
keeper := idempotency.New(storage)

ok, state, err := keeper.AttemptLock(ctx, "request-id-42")
if err != nil { return err }

if !ok {
    if state.Status == idempotency.StatusSuccess {
        return state.Data.(MyResponse), nil // cached
    }
    return ErrInProgress // someone else is processing
}

result, err := doWork(ctx)
if err != nil {
    _ = keeper.Delete(ctx, "request-id-42")
    return err
}
return keeper.Complete(ctx, "request-id-42", result, state)
```

### Redis (distributed)

```go
client := redis.NewClient(&redis.Options{Addr: redisAddr})
defer client.Close()

storage := redisstorage.New(client,
    redisstorage.WithTtl(time.Hour),
    redisstorage.WithKeyPrefix("myapp:idem:"),
)
keeper := idempotency.New(storage)
```

### NATS JetStream

```go
nc, _ := nats.Connect(natsURL)
js, _ := jetstream.New(nc)
defer nc.Drain()

storage, err := natsstorage.New(js,
    natsstorage.WithBucket("myapp-idempotency"),
    natsstorage.WithMaxAge(time.Hour),
)
if err != nil { return err }
keeper := idempotency.New(storage)
```

NATS bucket auto-enables `LimitMarkerTTL`, so per-key TTL on `AttemptLockWithTTL` works. **Requires NATS server 2.11+** — older servers fail
bucket creation with `ErrLimitMarkerTTLNotSupported` (visible at `New()`, not at runtime).

### Factory from YAML

```go
import idempfactory "github.com/altessa-s/go-atlas/data/idempotency/factory"

keeper, err := idempfactory.New(cfg.Idempotency).
    UseLogger(logger).
    UseRedisClient(redisClient).
    UseJetStream(js).
    UseScheduler(scheduler).
    UseCollector(collector).
    Build()
```

YAML:

```yaml
idempotency:
  ttl: 1h
  storage:
    type: redis
    redis:
      keysPrefix: "myapp:idem:"
```

Memory backend additionally accepts `storage.memory.cleanupSchedule` (cron string) so a scheduler periodically purges expired entries.

---

## Methods

| Method | Behavior |
|---|---|
| `AttemptLock(ctx, key)` | Atomic CAS claim. `(true, *State, nil)` on win (State carries CAS token); `(false, *State, nil)` on collision |
| `AttemptLockWithTTL(ctx, key, lockTtl)` | Same as AttemptLock with per-call lock TTL. `lockTtl=0` falls back to bucket default |
| `Complete(ctx, key, data, lockState)` | CAS-guarded write of success state. Returns `ErrLockStolen` if lock was taken over since AttemptLock |
| `CompleteWithTTL(ctx, key, data, lockState, resultTtl)` | Per-call result TTL. **NATS** returns `ErrPerCallTtlNotSupported` if `resultTtl > 0` |
| `Delete(ctx, key)` | Best-effort cleanup. Use on failure paths so the next attempt can re-claim |
| `SupportsAttemptLockWithTTL()` | Capability bit — true on every in-tree backend today |
| `SupportsCompleteWithTTL()` | Capability bit — `false` on NATS only |

`*State` returned by `AttemptLock` carries a private CAS token. Pass it back to `Complete` or `CompleteWithTTL` exactly as received; passing
`nil` returns `ErrMissingLockState` (the guard cannot be silently disabled).

---

## Stolen-lock detection

The classic bug it closes:

1. Node A: `AttemptLock(key)` → wins, starts processing.
2. Lock TTL expires while A is still working.
3. Node B: `AttemptLock(key)` → wins (the key is free again), starts its own processing.
4. A finishes the original work, calls `Complete(key, resultA)`.

Without a guard, step 4 silently overwrites B's state. Caller C reads back `resultA`, but the actual operation was performed by B — business
inconsistency.

`Complete` is CAS-guarded so step 4 returns `ErrLockStolen` instead. Per-backend mechanism:

| Backend | Mechanism |
|---|---|
| memory | mutex-protected byte comparison (current value vs lockToken) |
| redis | Lua script: `GET == lockToken ? SET : ErrLockStolen` |
| nats | `kv.Update(key, val, revision)` — server rejects on revision mismatch |

The token is opaque to callers — just pass the `*State` from `AttemptLock` back to `Complete`. The Keeper also injects a per-attempt UUID
nonce into the serialized state so two consecutive `AttemptLock` calls on the same key produce distinct bytes (memory and Redis CAS by byte
equality; NATS uses revision and ignores the nonce).

---

## Per-call TTL

Two TTLs, independent:

- **lock TTL** — how long a key stays locked in `InProgress` before another attempt can reclaim. Should be short (~max processing time × N retries).
- **result TTL** — how long the success state is cached for downstream callers. Should be long (whatever dedup window you actually need).

Typical webhook deduplication shape:

```go
ok, state, err := keeper.AttemptLockWithTTL(ctx, webhookID, 30*time.Second) // short lock
if err != nil { return err }
if !ok { return cached(state) }

result, err := process(ctx)
if err != nil { _ = keeper.Delete(ctx, webhookID); return err }

return keeper.CompleteWithTTL(ctx, webhookID, result, state, 24*time.Hour) // long result
```

Resolution order (highest precedence first):

1. Per-call `lockTtl` / `resultTtl` argument if positive
2. Backend-configured TTL (`memorystorage.WithTtl(d)` / `redisstorage.WithTtl(d)` / `natsstorage.WithMaxAge(d)`)
3. Hardcoded default (24 hours)

### NATS limitation

`nats.go` v1.51.0 doesn't expose per-message TTL on `KV.Put` or `KV.Update`. As a result:

| Method | NATS support |
|---|---|
| `AttemptLockWithTTL(... lockTtl > 0)` | ✓ via `jetstream.KeyTTL` on Create |
| `CompleteWithTTL(... resultTtl > 0)` | ✗ returns `ErrPerCallTtlNotSupported` |

For cross-backend code paths, branch on the capability bit:

```go
if keeper.SupportsCompleteWithTTL() {
    return keeper.CompleteWithTTL(ctx, key, result, state, 24*time.Hour)
}
return keeper.Complete(ctx, key, result, state) // bucket TTL
```

`AttemptLockWithTTL` works on every in-tree backend; the capability split exists because the limitation is Complete-side only.

---

## State and `Status`

```go
const (
    StatusInProgress Status = "IN_PROGRESS"
    StatusSuccess    Status = "SUCCESS"
)

type State struct {
    Status Status
    Data   any  // success payload — whatever you passed to Complete
}
```

On `AttemptLock` collision, `state.Status` is the answer to "what's happening with this key right now":

- `StatusInProgress` — another holder is still working. Block, retry, or return 409 to the user.
- `StatusSuccess` — already completed, `state.Data` is the cached result.

Callers don't mutate `*State` themselves — write through `Complete(data, state)` and `Keeper` builds the new `Success` payload.

---

## Errors

| Error | When |
|---|---|
| `ErrEmptyKey` | Empty key passed to AttemptLock/Complete/Delete — silently succeeding would disable dedupe |
| `ErrMissingLockState` | `Complete(... nil)` — the state returned by AttemptLock is required for the CAS guard |
| `ErrLockStolen` | Lock was taken over by another holder between AttemptLock and Complete |
| `ErrPerCallTtlNotSupported` | NATS `CompleteWithTTL(... resultTtl > 0)` — backend cannot honor per-call result TTL |

---

## Integration with transport layers

`go-atlas` ships HTTP and gRPC interceptors that wrap a `Keeper`:

- [`transport/http/server/middlewares/idempotency`](../../transport/http/server/middlewares/idempotency) — reads the configured header
  (default `Idempotency-Key`), short-circuits with `409 Conflict` for in-progress requests and `422 Unprocessable Entity` for already-used
  keys, captures the response body for replay.
- [`transport/grpc/interceptors/idempotency`](../../transport/grpc/interceptors/idempotency) — same model with metadata-based key extraction
  and gRPC status codes.

Both stash the `*State` returned by `AttemptLock` per-request and pass it to `Complete` in the post-handler hook so the stolen-lock guard
applies to real traffic, not just test code.

---

## Metrics

Subsystem `idempotency`:

| Metric | Type | Description |
|---|---|---|
| `idempotency_locks_acquired_total` | counter | Successful AttemptLock claims |
| `idempotency_locks_denied_total` | counter | AttemptLock collisions |
| `idempotency_completions_total` | counter | Successful Complete calls |
| `idempotency_deletions_total` | counter | Delete calls |
| `idempotency_errors_total` | counter | Storage errors from any operation |

Useful ratio: `denied / (denied + acquired)` is the duplicate-detection rate. A spike usually means upstream retries got more aggressive (or
broken).

---

## Failure modes

**`ErrLockStolen` from `Complete`.** Lock TTL expired during processing, another node took over. The current attempt's result is no longer
authoritative — drop it silently. Don't retry by re-acquiring; the new holder is doing the work. If this is frequent, raise the lock TTL or
shorten the operation.

**`ErrPerCallTtlNotSupported` from `CompleteWithTTL` on NATS.** You passed a positive `resultTtl` against the NATS backend. Either set
`resultTtl=0` (falls back to bucket TTL) or branch on `SupportsCompleteWithTTL()` to keep the call cross-backend.

**`State.Status == StatusInProgress` for very long.** Lock TTL is too long relative to the operation, or a holder crashed mid-processing.
Bucket TTL eventually frees the key; in the meantime callers see "in progress" indefinitely. Tune `lockTtl` to expected operation duration ×
small multiple.

**Memory backend "drift" between processes.** Memory storage is in-process — two instances of the same service won't see each other's locks.
Use Redis or NATS for distributed deployments. Memory is for tests and single-binary services.

**NATS bucket creation fails with `ErrLimitMarkerTTLNotSupported`.** Server is older than 2.11. Either upgrade the server or downgrade
go-atlas to a version before per-key TTL was wired in (not recommended — you lose `AttemptLockWithTTL`).

---

## Related docs

- [`docs/configuration.md`](../configuration.md) — overall YAML format and `cache_storage.yaml` template
- [`docs/metrics.md`](../metrics.md) — full metric reference for the repository
- [`data/idempotency/README.md`](../../data/idempotency/README.md) — godoc-level option reference
