# Idempotency keeper (`idempotency`)

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency"
```

A duplicate-request detector with a state machine. `AttemptLock` either claims a key (caller does the work) or returns the current `*State` of
whoever is already holding it (caller short-circuits with cached result or "in progress" signal). `Complete` writes the success state under a
CAS guard — a stale holder cannot overwrite a fresh one. Five pluggable backends: in-memory (real, with TTL), Redis, NATS JetStream KV, MongoDB, and
SQL (`database/sql`), where the MongoDB and SQL backends can join the caller's transaction.

---

## When to reach for `idempotency`

| Scenario | Use |
|----------|-----|
| HTTP / gRPC handler replaying a response for the same `Idempotency-Key` | `AttemptLock` + `Complete(data)` |
| Concurrent caller needs to see that a long-running operation is in progress | `AttemptLock` + check `State.Status == StatusInProgress` |
| Inspect the outcome of a previous run | `state.Status` enum + `state.Data` payload |
| An operation may outlast its lock | `Complete` returns `storages.ErrLockStolen` for a stale holder |
| Different lock lifetimes per key | `AttemptLockWithOpts` with `LockTTL` |
| Crashed holder leaves a lock | `MaxLockDuration` sets the takeover threshold or NATS lease cap |
| Same key reused for a different request | `AttemptLockOpts.Fingerprint` → `ErrFingerprintMismatch` |
| Replay is restricted to the recorded owner | `WithOwner` (+ `WithReplayAuthorizer`) → `ErrOwnerMismatch` |
| Key and business write must commit atomically | MongoDB or SQL backend inside the caller's transaction |
| Failure cleanup so the next attempt can reclaim the key | `Release` |

---

## Quick start

### Memory (tests, single-process)

```go
storage, err := memorystorage.New(memorystorage.WithTTL(time.Hour))
if err != nil { return err }
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
    _ = keeper.Release(ctx, "request-id-42", state)
    return err
}
return keeper.Complete(ctx, "request-id-42", result, state)
```

### Redis (distributed)

```go
client := redis.NewClient(&redis.Options{Addr: redisAddr})
defer client.Close()

storage := redisstorage.New(client,
    redisstorage.WithTTL(time.Hour),
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

`New` creates a file-backed bucket with per-key TTL enabled, which requires **NATS server 2.11+**. Existing buckets must allow per-key TTL
and keep one revision per key. When the bucket already exists with a different key TTL,
`New` returns `ErrBucketTTLMismatch` and leaves it untouched; pass `natsstorage.WithMigrateBucketTTL()` (YAML: `storage.nats.migrateBucketTTL: true`)
to update it deliberately. Migration rewrites the whole bucket config, not only the key TTL: the limit-marker TTL follows the new `MaxAge` too, and a
bucket created without one gets per-key TTL turned on, which needs NATS server 2.11+. New buckets are file-backed; a bucket that already exists with
another storage type fails with `ErrBucketStorageMismatch`, since the server cannot convert it.

### MongoDB (transactional)

```go
storage, err := mongostorage.New(db, mongostorage.WithTTL(time.Hour)) // creates the TTL index
if err != nil { return err }
keeper := idempotency.New(storage)
```

Called inside `mongo.Session.WithTransaction`, the lock and the result commit or abort together with the caller's own writes; see
[`storages/mongo`](../../reliability/idempotency/storages/mongo/README.md).

### SQL (transactional)

```go
storage, err := sqlstorage.New(db, sqlstorage.DialectPostgres) // PostgreSQL, or DialectMySQL for MySQL and MariaDB
if err != nil { return err }
defer storage.Close(ctx) // stops the purge loop
keeper := idempotency.New(storage)
```

Passed a transaction through `sqlstorage.WithTx(ctx, tx)` (or resolved from `store/postgres` through `WithTxResolver` / `UseSQLPool`), the lock and the
result commit or abort with the caller's writes. Expired keys are deleted by `PurgeExpired`, from a background loop every `WithPurgeInterval`; see
[`storages/sqldb`](../../reliability/idempotency/storages/sqldb/README.md).

### Factory from YAML

```go
import (
    idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
    idempfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
    redisfactory "github.com/altessa-s/go-atlas/factory/reliability/idempotency/storages/redis"
)

keeper, err := idempfactory.New(cfg.Idempotency).
    UseLogger(logger).
    UseStorage(idempotencyconfig.StorageTypeRedis, redisfactory.Builder(redisClient)).
    UseScheduler(scheduler).
    UseMeter(collector).
    Build(ctx)
```

The factory builds the memory storage and, over the `*sql.DB` given to `UseSQLDB` (or `UseSQLPool`), the `sqldb` one. The `redis`, `nats` and `mongo`
storages register with `UseStorage` from `factory/reliability/idempotency/storages/<backend>` (`redisfactory.Builder(client)`,
`natsfactory.Builder(js)`, `mongofactory.Builder(db)`); a configured type without one fails `Build` naming the package to add.

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

`storage.type` is one of `memory`, `redis`, `nats`, `mongo` or `sqldb`. Memory backend additionally accepts `storage.memory.cleanupSchedule` (cron
string) so a scheduler periodically purges expired entries; the mongo backend accepts `storage.mongo.collection` (default `idempotency_keys`).

---

## Methods

| Method | Behavior |
|--------|----------|
| `AttemptLock(ctx, key)` | Atomic claim. Returns `(true, *State, nil)` with a CAS token on success or `(false, *State, nil)` on collision. |
| `AttemptLockWithOpts(ctx, key, opts)` | Overrides `LockTTL`, `MaxLockDuration`, and `Fingerprint`. Zero values use backend or Keeper defaults. |
| `Complete(ctx, key, data, lockState)` | Stores success using CAS. Returns `storages.ErrLockStolen` if the caller lost the lock. |
| `Release(ctx, key, lockState)` | Releases the caller's lock for another attempt. Returns `storages.ErrLockStolen` if the caller lost the lock. |
| `Delete(ctx, key)` | Removes the key unconditionally for administrative invalidation. |

`*State` returned by `AttemptLock` carries a private CAS token. Pass it back to `Complete` exactly as received; passing `nil` returns
`storages.ErrMissingLockState` (the guard cannot be silently disabled).

---

## Stolen-lock detection

The classic bug it closes:

1. Node A: `AttemptLock(key)` → wins, starts processing.
2. Lock TTL expires while A is still working.
3. Node B: `AttemptLock(key)` → wins (the key is free again), starts its own processing.
4. A finishes the original work, calls `Complete(key, resultA)`.

Without a guard, step 4 silently overwrites B's state. Caller C reads back `resultA`, but the actual operation was performed by B — business
inconsistency.

`Complete` is CAS-guarded so step 4 returns `storages.ErrLockStolen` instead. Per-backend mechanism:

| Backend | Mechanism                                                             |
|---------|-----------------------------------------------------------------------|
| memory  | mutex-protected byte comparison (current value vs lockToken)          |
| redis   | Lua script: `GET == lockToken ? SET : ErrLockStolen`                  |
| nats    | `kv.Update(key, val, revision)` — server rejects on revision mismatch |
| mongo   | `UpdateOne` filtered on the stored bytes                              |
| sqldb   | Conditional SQL writes comparing live stored bytes (revision column on MySQL) |

The token is opaque to callers — just pass the `*State` from `AttemptLock` back to `Complete`. The Keeper also injects a per-attempt UUID
nonce into the serialized state so two consecutive `AttemptLock` calls on the same key produce distinct bytes (memory and Redis CAS by byte
equality, as MongoDB does; NATS uses revision and ignores the nonce).

---

## Per-call overrides

`AttemptLockWithOpts(ctx, key, opts)` accepts a struct of per-call overrides. All fields are optional; zero values fall back to defaults.

```go
ok, state, err := keeper.AttemptLockWithOpts(ctx, key, idempotency.AttemptLockOpts{
    LockTTL:         30 * time.Second, // override storage TTL for this lock
    MaxLockDuration: 30 * time.Second, // tighter orphan-reclaim threshold
})
```

| Field             | Resolution order (first match wins)                                                                                  |
|-------------------|----------------------------------------------------------------------------------------------------------------------|
| `LockTTL`         | per-call value > backend `WithTTL` / `WithMaxAge` > 24h hard default                                                 |
| `MaxLockDuration` | per-call value > Keeper `WithMaxLockDuration` > `DefaultMaxLockDuration` (5m)                                        |
| `Fingerprint`     | per-call value; nil records and checks no fingerprint (see [Request fingerprints](#request-fingerprints-and-owners)) |

Useful when different keys within one Keeper need different lifetimes — short-lived OTP tokens vs long-running webhook processing — or when a
slow operation genuinely needs more headroom before the orphan-steal kicks in.

The backend configuration controls the lifetime of a completed result. With NATS KV, `Complete` starts a fresh bucket TTL for the result.

## Request fingerprints and owners

A key identifies one command. `core/encoding/keys.Digest(parts...)` hashes the operation and a canonical encoding of the request; pass it as
`AttemptLockOpts.Fingerprint` and it is stored with the key. A later attempt with the same key and another fingerprint fails with
`ErrFingerprintMismatch` instead of receiving the stored result. Records or callers without a fingerprint are not checked.

`WithOwner(func(ctx) string)` records an opaque owner label with each key. A caller with another label gets `ErrOwnerMismatch` and nothing of the
record. A `Keeper` built without `WithOwner` (and without `WithReplayAuthorizer`) cannot replay or take over a record that has a persisted owner:
collisions return `ErrOwnerMismatch`, and age-based reclaim refuses a live lock owned by another caller. The optional
`WithReplayAuthorizer(func(ctx, owner) error)` decides every replay using the recorded label. The Keeper wraps its error in `ErrOwnerMismatch`.
Return the service's NotFound error to hide the record. A live orphaned lock can only be replaced by its owner. Expiry ends the owner and fingerprint
bindings. The `factory/reliability/idempotency` package defaults `UseOwner` to `principalowner.PrincipalOwner`.
See the [package README](../../reliability/idempotency/README.md#owners) for details.

### NATS lock leases

The acquiring call sets the lease. The Keeper uses its resolved `MaxLockDuration` or a shorter positive `LockTTL`. The storage writes that duration as
a per-key TTL rounded up to whole seconds, and JetStream also applies the bucket TTL. To enable per-key TTL on an existing bucket, set
`storage.nats.migrateBucketTTL: true` or pass `WithMigrateBucketTTL()` to the storage constructor.

After the lease expires, another owner can acquire and complete the same key. With the default owner check, the original owner then gets
`ErrOwnerMismatch` on retry until the completed record expires.

---

## Startup warning when `MaxLockDuration` is unset

`Keeper.New` emits a single `slog.Warn` when no `WithMaxLockDuration` option is passed. The intent is **soft acknowledgment** — make every
service owner think about whether the package default fits their workload, without breaking on-boarding the way Required-validation would.

The warning message is `idempotency: using default MaxLockDuration; review for your service`. It reports `default=5m0s` and suggests passing
`idempotency.WithMaxLockDuration(d)` to acknowledge or override the default.

### Why this needs a deliberate choice

`MaxLockDuration` is the hinge of the [orphan-lock reclaim](#orphan-lock-reclaim) mechanism: the threshold past which a stale InProgress entry
is treated as abandoned and forcibly reclaimed by the next `AttemptLock`. Picking it wrong has asymmetric costs:

- **Too short for real handler runtimes.** A second caller may acquire the key while the first is still working. The first caller's `Complete`
  returns `storages.ErrLockStolen`, but both callers can already have produced side effects, such as sending the same webhook twice.
- **Too long for the retry window.** When `MaxLockDuration` exceeds the broker's retry window (for example, JetStream `AckWait × MaxDeliver`), the
  broker can give up before another attempt can acquire the key. An age-aware store may then retain the lock until its storage TTL expires.

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

Factory users set `idempotency.maxLockDuration` in YAML — `Build(ctx)` forwards it via `WithMaxLockDuration` only when set, so leaving the field
empty intentionally surfaces the warning at boot. Keep `maxLockDuration` empty to flag "this service has not been tuned yet"; fill it in
after evaluating handler runtime + upstream retry window.

---

## Orphan-lock reclaim

If a holder crashes before calling `Complete` or `Release`, the lock remains until its TTL expires or a retry takes it over.
On an age-aware backend, when a caller collides with its own in-progress record, the Keeper can call `Steal` with the observed bytes, replacement bytes,
the resolved `MaxLockDuration`, and the contender's `LockTTL`. A Keeper skips that attempt when its own acquisition stamp shows the
lock is still fresh. For Redis, MongoDB, SQL, and memory, the store stamps acquisition and decides whether the lock's age strictly
exceeds the threshold. A positive lock TTL applies to the replacement, and a nonpositive TTL uses the store default.

`Steal` returns `(stolen, current, token, err)`. A successful takeover returns `stolen=true` and a fresh ownership token. A refusal returns
the live value in `current` when the key still exists and leaves `token` nil. The Keeper checks the returned record before exposing the
state to the caller. After takeover, the previous holder's token fails with `storages.ErrLockStolen` on completion or release.

| Backend | Age and replacement |
|---------|---------------------|
| memory  | The configured clock supplies age. A mutex protects the live-lock, byte comparison, and replacement. |
| redis   | Lua uses Redis `TIME` and checks the live lock and expected bytes before replacing it with the contender's TTL. |
| nats    | JetStream expires the lease set by the acquiring call. A retry acquires a fresh KV revision. |
| mongo   | A conditional update checks the live lock, expected bytes, and age using the server's `$$NOW`. |
| sqldb   | A conditional update checks the live lock, expected bytes, and age using the database statement clock. |

NATS `Steal` returns the current value without replacing a live record. Wrappers must forward `storages.LockLeaseStorage` for the Keeper to
apply the lease cap. A holder whose lease has expired receives `ErrLockStolen` on completion or release.

Tune via:

| Setting                                 | Effect                                                                                |
|-----------------------------------------|---------------------------------------------------------------------------------------|
| `WithMaxLockDuration(d)` (default `5m`) | Keeper-wide age threshold or acquisition-time lease cap                               |
| `AttemptLockOpts{MaxLockDuration: d}`   | Per-call override (positive value wins over Keeper default)                           |
| `WithMaxLockDuration(d)` with `d <= 0`  | Ignored — `New` falls back to `5m`; reclaim cannot be disabled through the public API |

Reclaim runs on an `AttemptLock` retry. JetStream expires NATS leases even when no caller retries.

---

## State and `idempotency.Status`

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

- `idempotency.StatusInProgress` — another holder is still working. Block, retry, or return 409 to the user.
- `idempotency.StatusSuccess` — already completed, `state.Data` is the cached result.

Callers don't mutate `*State` themselves — write through `Complete(data, state)` and `Keeper` builds the new `Success` payload.

---

## Errors

| Error                    | When                                                                                                        |
|--------------------------|-------------------------------------------------------------------------------------------------------------|
| `storages.ErrEmptyKey`            | Empty key passed to AttemptLock/Complete/Release/Delete — silently succeeding would disable dedupe          |
| `storages.ErrMissingLockState`    | `Complete(... nil)` or `Release(... nil)` — the state returned by AttemptLock is required for the CAS guard |
| `storages.ErrLockStolen`          | The caller’s ownership token expired or was superseded before Complete/Release                              |
| `ErrFingerprintMismatch` | The key was recorded with another request fingerprint                                                       |
| `ErrOwnerMismatch`       | The key belongs to another owner, or the `ReplayAuthorizer` refused the replay (its error is wrapped)       |

---

## Integration with transport layers

`go-atlas` ships HTTP and gRPC interceptors that wrap a `Keeper`:

- `transport/http/server/middlewares/idempotency` — reads the configured header (default `Idempotency-Key`), short-circuits with `409 Conflict` for
  in-progress requests, `422 Unprocessable Entity` for already-used keys and fingerprint mismatches, and `403 Forbidden` for another owner's key (or
  the `HTTPStatus()` of a `ReplayAuthorizer` error). In dependency-ordered HTTP chains, idempotency runs after `auth` and `mtls` so the keeper can
  resolve the authenticated owner. Response replay is off by default and enabled with `WithResponseReplay()`, failing closed unless the record has a
  persisted owner; entity-ID extraction and echoing (`WithExposeEntityID()`) likewise requires an owner-bound record.
- `transport/grpc/interceptors/idempotency` — same model with metadata-based key extraction and gRPC status codes (`InvalidArgument` for a
  fingerprint mismatch, `PermissionDenied` for another owner's key, or the status of a `ReplayAuthorizer` error). Ships a client-side
  counterpart in the same package: `DeriveKey(seed, call)` mints a deterministic UUID v4 anchored to the package namespace, `WithKey` /
  `WithDerivedKey` attach the key to the outgoing metadata, and `UnaryClientInterceptor` stamps a derived key on every call whose context
  was tagged with `WithOperation(ctx, operationID)`. The server scopes idempotency per service, so the `call` component (typically
  `info.FullMethod`) keeps sibling downstream calls within one operation from colliding.

Both stash the `*State` returned by `AttemptLock` per-request and pass it to `Complete` in the post-handler hook so the stolen-lock guard
applies to real traffic, not just test code.

---

## Metrics

Subsystem `idempotency`:

| Metric                             | Type    | Description                       |
|------------------------------------|---------|-----------------------------------|
| `idempotency_locks_acquired_total` | counter | Successful AttemptLock claims     |
| `idempotency_locks_denied_total`   | counter | AttemptLock collisions            |
| `idempotency_completions_total`    | counter | Successful Complete calls         |
| `idempotency_deletions_total`      | counter | Delete calls                      |
| `idempotency_errors_total`         | counter | Storage errors from any operation |

Useful ratio: `denied / (denied + acquired)` is the duplicate-detection rate. A spike usually means upstream retries got more aggressive (or broken).

---

## Failure modes

**`storages.ErrLockStolen` from `Complete`.** Lock TTL expired during processing, another node took over. The current attempt's result is no longer
authoritative — drop it silently. Don't retry by re-acquiring; the new holder is doing the work. If this is frequent, raise the lock TTL or
shorten the operation.

**`State.Status == StatusInProgress` for very long.** Lock TTL is too long relative to the operation, or a holder crashed mid-processing. Tune
`WithMaxLockDuration` (or per-call `MaxLockDuration`) based on expected operation duration. Age-aware stores can reclaim an eligible lock on
collision. For NATS, the original acquisition sets the lease, and a retry can acquire the key after expiry. Until then, callers see "in
progress" until either a retry triggers reclaim or the bucket TTL fires.

**Memory backend "drift" between processes.** Memory storage is in-process — two instances of the same service won't see each other's locks.
Use Redis, NATS or MongoDB for distributed deployments. Memory is for tests and single-binary services.

**NATS bucket creation fails with `ErrLimitMarkerTTLNotSupported`.** Server is older than 2.11, which per-key TTL (`AttemptLockWithTTL`) needs.
Upgrade the server.

---

## Related docs

- [`docs/configuration.md`](../configuration.md) — overall YAML format and `cache_storage.yaml` template
- [`docs/metrics.md`](../metrics.md) — full metric reference for the repository
