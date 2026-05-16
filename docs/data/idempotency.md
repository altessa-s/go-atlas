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
| Different lock lifetime per key (short OTP vs long webhook) | `AttemptLockWithOpts` with `LockTTL` |
| Crashed holder leaves an InProgress entry — unblock retries before bucket TTL fires | `WithMaxLockDuration` (or per-call `MaxLockDuration`) — orphan-lock CAS-steal |
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
  maxLockDuration: 2m   # silences the startup warning; pick a value tuned for your handler
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
| `AttemptLockWithOpts(ctx, key, opts)` | Same as AttemptLock with per-call overrides — `LockTTL` (lock lifetime) and `MaxLockDuration` (orphan-reclaim threshold). Zero values fall back to backend / Keeper defaults |
| `Complete(ctx, key, data, lockState)` | CAS-guarded write of success state. Returns `ErrLockStolen` if lock was taken over since AttemptLock |
| `Delete(ctx, key)` | Best-effort cleanup. Use on failure paths so the next attempt can re-claim |

`*State` returned by `AttemptLock` carries a private CAS token. Pass it back to `Complete` exactly as received; passing `nil` returns
`ErrMissingLockState` (the guard cannot be silently disabled).

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

## Per-call overrides

`AttemptLockWithOpts(ctx, key, opts)` accepts a struct of per-call overrides. Both fields are optional; zero values fall back to defaults.

```go
ok, state, err := keeper.AttemptLockWithOpts(ctx, key, idempotency.AttemptLockOpts{
    LockTTL:         30 * time.Second, // override storage TTL for this lock
    MaxLockDuration: 30 * time.Second, // tighter orphan-reclaim threshold
})
```

| Field             | Resolution order (first match wins)                                |
|-------------------|--------------------------------------------------------------------|
| `LockTTL`         | per-call value > backend `WithTtl` / `WithMaxAge` > 24h hard default |
| `MaxLockDuration` | per-call value > Keeper `WithMaxLockDuration` > `DefaultMaxLockDuration` (5m) |

Useful when different keys within one Keeper need different lifetimes — short-lived OTP tokens vs long-running webhook processing — or when a
slow operation genuinely needs more headroom before the orphan-steal kicks in.

Result TTL (lifetime of the success state after `Complete`) is fixed per Keeper instance via the backend-level option. If your operation
genuinely needs `lock=30s, result=24h` style dual-TTL semantics, file an issue with the use case — the dual-TTL API was added speculatively
and removed because no in-tree caller exercised it.

### NATS lock-TTL requirement

NATS per-call `LockTTL > 0` works via `jetstream.KeyTTL` on Create — but the bucket must be created with `LimitMarkerTTL` (handled
automatically in `storages/nats.New`).
This requires **NATS server 2.11+**. Older servers fail bucket creation at `New()` time with `ErrLimitMarkerTTLNotSupported`.

---

## Startup warning when `MaxLockDuration` is unset

`Keeper.New` emits a single `slog.Warn` when no `WithMaxLockDuration` option is passed. The intent is **soft acknowledgment** — make every
service owner think about whether the package default fits their workload, without breaking on-boarding the way Required-validation would.

```
level=WARN msg="idempotency: using default MaxLockDuration; review for your service" default=5m0s fix="pass idempotency.WithMaxLockDuration(d) to acknowledge or override"
```

### Why this needs a deliberate choice

`MaxLockDuration` is the hinge of the [orphan-lock reclaim](#orphan-lock-reclaim) mechanism: the threshold past which a stale InProgress entry
is treated as abandoned and forcibly reclaimed by the next `AttemptLock`. Picking it wrong has asymmetric costs:

| Too short relative to real handler p99 | Too long relative to retry window |
|---|---|
| **False-positive steal.** Holder A is still legitimately working when caller B reclaims the key and starts its own work. A finishes, calls `Complete` → `ErrLockStolen` (correctly rejected), but B re-runs the same operation. **Duplicate side-effects** — second SMS, second bank call, second webhook delivery. Worse than the original "stuck in-progress" symptom. | **Reclaim never fires.** If `MaxLockDuration` exceeds the broker's retry window (e.g. NATS JetStream `AckWait × MaxDeliver`), the broker has already given up and dropped the message before any caller would have triggered reclaim. The orphan then waits out the storage bucket TTL — typically 24h — exactly the failure mode this feature was meant to fix. |

The "right" value is service-specific: it depends on the realistic upper bound of handler runtime *and* the retry window of whatever upstream
is calling in. A service with synchronous HTTP traffic and 30s client timeouts wants something close to 30-60s. A worker behind a NATS
consumer with `AckWait=30s, MaxDeliver=10` wants something around the resulting ~5min retry window. A long-running workflow calling out to
slow KYC providers may legitimately need 10-15 min.

### Why 5 minutes as the package default

The `DefaultMaxLockDuration = 5 * time.Minute` was picked as a reasonable middle ground for the "no one tuned this" case:

- **Above the p99 of typical HTTP/webhook handlers.** Most synchronous handlers complete in <30s. Slow externals (PSP/KYC/SMS gateways) can
  push handlers to 2-3 min, but anything beyond that is uncommon. 5 min leaves headroom for the long tail without being absurd.
- **Matches default NATS JetStream retry windows.** A consumer with `AckWait=30s, MaxDeliver=10` retries for ~5 min before sending the
  message to DLQ or dropping it. Reclaim past that point is futile — no caller will retry to trigger it.
- **Below typical bucket TTLs.** Default storage TTL is 24h, so an unreclaimed orphan is always cleaned up *eventually*. 5 min is the point
  where "wait for the next retry to reclaim" beats "wait for bucket TTL".

It is **not** universally correct, which is why the warning exists — to shift the cost of the wrong default from "silent production bug" to
"one log line at boot that nudges you to evaluate".

### Alternatives considered (and why they were rejected)

- **Required validation** (`WithMaxLockDuration` mandatory; `New` returns an error or panics when unset). Rejected: forces every caller —
  including tests, factory configs, and downstream services on upgrade — to write the same value. When ~90% of callers want the same number,
  that number should be the default. Required would break on-boarding to score a small reduction in false-positive steal risk.
- **Sentinel + lazy resolution without warning.** Functionally identical to a default, but loses the "make teams think" property this whole
  change was about. The warning is the entire point.
- **Per-process dedup of the warning.** Rejected as unnecessary — one or two Keepers per service produces one or two log lines, which is
  exactly the desired signal.

### Silencing the warning

Pass **any** explicit positive value to `WithMaxLockDuration`:

```go
keeper := idempotency.New(storage,
    idempotency.WithMaxLockDuration(2*time.Minute), // tuned for this service
)

// or, if 5m is genuinely fine after evaluating the tradeoffs above:
keeper := idempotency.New(storage,
    idempotency.WithMaxLockDuration(idempotency.DefaultMaxLockDuration), // explicit acknowledgment
)
```

Both forms silence the warning. The second one is the "I read the docs and 5m fits my service" idiom — code review can grep for it to confirm
the choice was deliberate.

Factory users set `idempotency.maxLockDuration` in YAML — `Build()` forwards it via `WithMaxLockDuration` only when set, so leaving the field
empty intentionally surfaces the warning at boot. Keep `maxLockDuration` empty to flag "this service has not been tuned yet"; fill it in
after evaluating handler runtime + upstream retry window.

---

## Orphan-lock reclaim

The other classic bug: a holder crashes between `AttemptLock` and `Complete`/`Delete`. The InProgress entry sits in storage until the bucket
TTL fires (default 24h), and every retry sees "in progress" even though no one is processing.

Every InProgress wire embeds a `LockedAt` timestamp. On `AttemptLock` collision, if the existing entry has `Status == InProgress` and is
older than the resolved `MaxLockDuration`, the Keeper calls the storage layer's `Steal` primitive to atomically replace the orphan with a
fresh wire — and the new holder gets a fresh CAS token.

The crashed holder's stale `Complete` then surfaces `ErrLockStolen` (its CAS token is no longer current), so callers see the normal "we lost
the race" path and skip writing.

Per-backend Steal mechanism:

| Backend | Mechanism |
|---|---|
| memory | mutex + `bytes.Equal` + replace |
| redis | Lua script: `GET == expectedVal ? SET PX ttl : ErrLockStolen` |
| nats | `kv.Get` → `bytes.Equal` → `kv.Update(key, val, revision)` (revision check protects the Get/Update gap) |

Tune via:

| Setting                                 | Effect                                                       |
|-----------------------------------------|--------------------------------------------------------------|
| `WithMaxLockDuration(d)` (default `5m`) | Keeper-wide threshold for collision-time orphan-reclaim      |
| `AttemptLockOpts{MaxLockDuration: d}`   | Per-call override (positive value wins over Keeper default)  |
| Keeper field set to `0`                 | Disables reclaim entirely — all collisions surface as in-progress |

Reclaim only triggers on the *next* `AttemptLock` collision; there's no background sweeper. If no caller retries the key, the entry remains
until the bucket TTL fires (which is the original failure mode without this feature).

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

---

## Integration with transport layers

`go-atlas` ships HTTP and gRPC interceptors that wrap a `Keeper`:

- `transport/http/server/middlewares/idempotency` — reads the configured header (default `Idempotency-Key`), short-circuits with `409 Conflict`
  for in-progress requests and `422 Unprocessable Entity` for already-used keys, captures the response body for replay.
- `transport/grpc/interceptors/idempotency` — same model with metadata-based key extraction and gRPC status codes.

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

Useful ratio: `denied / (denied + acquired)` is the duplicate-detection rate. A spike usually means upstream retries got more aggressive (or broken).

---

## Failure modes

**`ErrLockStolen` from `Complete`.** Lock TTL expired during processing, another node took over. The current attempt's result is no longer
authoritative — drop it silently. Don't retry by re-acquiring; the new holder is doing the work. If this is frequent, raise the lock TTL or
shorten the operation.

**`State.Status == StatusInProgress` for very long.** Lock TTL is too long relative to the operation, or a holder crashed mid-processing. Tune
`WithMaxLockDuration` (or per-call `MaxLockDuration`) to expected operation duration × small multiple — the next AttemptLock past that
threshold reclaims the orphan via CAS-steal. If no caller retries, the bucket TTL eventually frees the key; in the meantime callers see "in
progress" until either a retry triggers reclaim or the bucket TTL fires.

**Memory backend "drift" between processes.** Memory storage is in-process — two instances of the same service won't see each other's locks.
Use Redis or NATS for distributed deployments. Memory is for tests and single-binary services.

**NATS bucket creation fails with `ErrLimitMarkerTTLNotSupported`.** Server is older than 2.11. Either upgrade the server or downgrade go-atlas
to a version before per-key TTL was wired in (not recommended — you lose `AttemptLockWithTTL`).

---

## Related docs

- [`docs/configuration.md`](../configuration.md) — overall YAML format and `cache_storage.yaml` template
- [`docs/metrics.md`](../metrics.md) — full metric reference for the repository
