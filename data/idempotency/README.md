# idempotency

```go
import "github.com/altessa-s/go-atlas/data/idempotency"
```

Package `idempotency` provides duplicate request detection using idempotency keys. Supports multiple storage backends (memory, Redis, NATS,
MongoDB) for single-instance and distributed systems; the [MongoDB backend](storages/mongo) joins the caller's transaction, so a key commits
atomically with the business change.

## Tenant isolation (multi-tenant deployments)

The idempotency key is global unless you scope it. Two tenants that submit the **same** bare key collide in one keyspace, so a tenant that knows or
guesses another tenant's key can be served that tenant's completed response (`Complete` stores the response `Data`, and a subsequent `AttemptLock`
returns it). In a multi-tenant service this is a cross-tenant data leak.

**Contract:** scope keys to the tenant/subject. Either prefix every key you pass, or configure `WithKeyNamespace` so the `Keeper` prefixes every storage
operation (`AttemptLock`, `Steal`, `Complete`, `Release`, `Delete`) automatically:

```go
k := idempotency.New(storage,
    idempotency.WithMaxLockDuration(2*time.Minute),
    idempotency.WithKeyNamespace(func(ctx context.Context) string {
        return principal.FromContext(ctx).Tenant() // "" disables prefixing (backward-compatible default)
    }),
)
```

Returning `""` leaves keys unchanged, so existing single-tenant callers are unaffected.

## Request fingerprints

A key identifies one command. Pass a fingerprint of the request with `AttemptLockOpts.Fingerprint` — `Fingerprint(parts...)` hashes the
operation and a canonical encoding of the request, e.g. a deterministic protobuf marshal — and it is stored with the key. A later attempt with
the same key and another fingerprint fails with `ErrFingerprintMismatch` instead of receiving the stored result; transports answer it with 422
(HTTP) or `InvalidArgument` (gRPC). Records or callers without a fingerprint are not checked.

```go
fp := idempotency.Fingerprint([]byte(method), canonicalBody)
ok, state, err := keeper.AttemptLockWithOpts(ctx, key, idempotency.AttemptLockOpts{Fingerprint: fp})
if errors.Is(err, idempotency.ErrFingerprintMismatch) { /* the key was reused for another command */ }
```

## Owners

`WithKeyNamespace` separates keyspaces. Within one namespace, `WithOwner(func(ctx) string)` records an owner label with each key — a
subject, or whatever access attributes the service needs later (a legal entity and an employee, encoded). The label is opaque to the
Keeper:

- without a `ReplayAuthorizer`, a caller with another label gets `ErrOwnerMismatch` and nothing of the record — neither its result nor
  whether its fingerprint matches;
- with `WithReplayAuthorizer(func(ctx, owner) error)`, the authorizer alone decides every replay from the recorded label, so the replay
  goes to whoever may read the record. Its error is returned wrapped in `ErrOwnerMismatch`: return the service's NotFound error to hide an
  invisible record — `errors.Is` still matches it, the gRPC interceptor answers with its status and the HTTP middleware with its
  `HTTPStatus()`.

Only the record's own owner may take over its orphaned lock. Records written without an owner are not checked unless an authorizer is set.

## Stolen-lock detection

`AttemptLock` returns a `*State` carrying an opaque CAS token. `Complete` requires that same `*State` back; if the lock has been taken over by another
holder between `AttemptLock` and `Complete` (typically because the lock TTL fired during processing), Complete returns `ErrLockStolen` instead of
silently overwriting the new holder's result.

Backend-specific CAS primitives:

| Backend | Mechanism |
|---|---|
| memory | mutex-protected byte comparison of stored value vs lockToken |
| redis | Lua script: `GET == lockToken ? SET : ErrLockStolen` |
| nats | `kv.Update(key, val, revision)` — server rejects on revision mismatch |
| mongo | `UpdateOne` filtered on the stored bytes; a lock is one atomic upsert |

Pass the `*State` from AttemptLock straight back to Complete:

```go
ok, state, err := keeper.AttemptLock(ctx, key)
if err != nil { return err }
if !ok { /* in-progress or completed; consult state */ return nil }
result, err := doWork(ctx)
if err != nil {
    _ = keeper.Release(ctx, key, state) // release only our own lock
    return err
}
return keeper.Complete(ctx, key, result, state)  // state carries the CAS token
```

Passing `nil` state to Complete returns `ErrMissingLockState` — the guard cannot be silently disabled.

## Per-call overrides

`AttemptLockWithOpts(ctx, key, opts)` accepts a struct of per-call overrides:

```go
ok, state, err := keeper.AttemptLockWithOpts(ctx, key, idempotency.AttemptLockOpts{
    LockTTL:         30 * time.Second, // override backend default for this lock
    MaxLockDuration: 30 * time.Second, // tighter orphan-reclaim threshold
})
```

| Field             | Zero-value behavior                                                  |
|-------------------|----------------------------------------------------------------------|
| `LockTTL`         | Use the backend's configured TTL (`WithTTL` / `WithMaxAge` / YAML).  |
| `MaxLockDuration` | Use the Keeper's configured value (`WithMaxLockDuration`, default 5m). |
| `Fingerprint`     | No fingerprint is recorded or checked.                               |

Useful when different keys legitimately need different lock lifetimes within one Keeper instance — e.g. short-lived OTP tokens vs longer broker-driven
webhook processing.

Result TTL (how long the cached success state lives) is configured at the backend level. If you need per-call result-TTL control, file an issue with the
use case — the dual-TTL API was tried and removed because no in-tree caller exercised it.

## Startup warning when MaxLockDuration is unset

`New` emits a single `slog.Warn` when no `WithMaxLockDuration` is passed:

```
level=WARN msg="idempotency: using default MaxLockDuration; review for your service" default=5m0s fix="pass idempotency.WithMaxLockDuration(d) to acknowledge or override"
```

5 minutes is a defensible default but **not** universally correct — a slow handler that legitimately runs longer would be reclaimed mid-flight,
producing duplicate side-effects. Service owners should evaluate it explicitly:

```go
keeper := idempotency.New(storage,
    idempotency.WithMaxLockDuration(2*time.Minute), // tuned for this service
)

// or, if 5m is genuinely fine:
keeper := idempotency.New(storage,
    idempotency.WithMaxLockDuration(idempotency.DefaultMaxLockDuration), // explicit acknowledgment
)
```

Either form silences the warning. Factory users set `idempotency.maxLockDuration` in YAML to achieve the same.

## Orphan-lock reclaim

When a holder crashes between `AttemptLock` and `Complete`/`Release`, the InProgress entry sits in storage until the bucket TTL fires (default 24h). To
unblock retries sooner, every InProgress wire embeds a `LockedAt` timestamp. On collision, if the existing entry is older than the resolved
`MaxLockDuration`, `AttemptLock` issues a CAS-steal via the storage layer and the new holder gets a fresh lock token.

| Behavior                          | Setting                                       |
|-----------------------------------|-----------------------------------------------|
| Default reclaim threshold         | 5 minutes (`DefaultMaxLockDuration`).         |
| Override Keeper-wide              | `idempotency.WithMaxLockDuration(10*time.Minute)`. |
| Override per-call                 | `AttemptLockOpts{MaxLockDuration: 30*time.Second}`. |
| Disable reclaim                   | Set the Keeper field to 0 — collisions surface as in-progress. |

The crashed holder's stale `Complete` then surfaces `ErrLockStolen` because its CAS token is no longer current. Callers see this as the normal "we lost
the race" path and skip writing.

## Options

| Option                  | Default                  | Description                                                |
|-------------------------|--------------------------|------------------------------------------------------------|
| `WithLogger`            | discard                  | Structured logger                                          |
| `WithSerializer`        | JSON                     | Serialization format                                       |
| `WithCollector`         | none                     | Metrics collector                                          |
| `WithMaxLockDuration`   | `DefaultMaxLockDuration` (5m) | Threshold for orphan-lock CAS-steal on AttemptLock collision |
| `WithKeyNamespace`      | none                     | Per-context key prefix (tenant / subject)                  |
| `WithOwner`             | none                     | Caller identity recorded with each key (see Owners)        |
| `WithReplayAuthorizer`  | none (refuse)            | Lets chosen callers see records of other owners            |

## Subpackages

| Package                                  | Description                          |
|------------------------------------------|--------------------------------------|
| [factory](./factory)                     | Configuration-based creation         |
| [storages/memory](./storages/memory)     | In-memory backend with TTL           |
| [storages/redis](./storages/redis)       | Distributed Redis storage            |
| [storages/nats](./storages/nats)         | NATS JetStream storage               |
| [storages/mongo](./storages/mongo)       | MongoDB storage, joins transactions  |

## Ownership-safe release

`Release(ctx, key, state)` requires the token returned by `AttemptLock`, just like `Complete`. A missing token returns `ErrMissingLockState`; an
expired, completed or replaced lock returns `ErrLockStolen`. HTTP and gRPC adapters use `Release` when a handler fails. `Delete` unconditionally removes
a key and is reserved for administrative invalidation, never request cleanup.
