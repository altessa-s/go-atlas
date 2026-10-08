# mongo

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages/mongo"
```

Package `mongo` is a MongoDB [`storages.Storage`](../README.md) for [`idempotency`](../../README.md) keys. A key is one document
`{_id: key, val: <state>, expires_at: <date>}`: `AttemptLock` takes it with one atomic upsert when it is missing or expired, and `Complete`,
`Steal` and `Release` compare and set on the stored bytes, as the memory and Redis backends do. A TTL index on `expires_at` removes old keys;
since the TTL monitor runs about once a minute, every operation also treats an expired document as absent. Expiry uses the server clock.

## Options

| Option                   | Default            | Description                                              |
|--------------------------|--------------------|----------------------------------------------------------|
| `WithCollectionName`     | `idempotency_keys` | Collection the keys are stored in (`New` only)           |
| `WithTTL`                | 24h                | Lifetime of a result and the default lifetime of a lock  |
| `WithContext`            | background         | Context for the TTL index creation                       |
| `WithIndexCreateTimeout` | 10s                | Timeout for the TTL index creation                       |

`New(db, ...)` and `NewWithCollection(coll, ...)` create the TTL index and fail when they cannot.

## Transactions

Every operation runs on the context it is given. Inside `mongo.Session.WithTransaction` the lock and the result commit or abort with the
caller's own writes, so recording the command and applying it are one atomic step:

- an aborted transaction leaves no lock behind — no `Release` is needed;
- a duplicate racing in its own transaction hits a write conflict, `WithTransaction` retries it, and it then sees the committed key;
- an uncommitted lock is invisible to other requests, which therefore wait on the write conflict rather than receive `IN_PROGRESS`.

```go
_, err := sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
    ok, held, err := keeper.AttemptLockWithOpts(ctx, key, idempotency.AttemptLockOpts{Fingerprint: fp})
    if err != nil || !ok {
        return nil, err // ErrFingerprintMismatch, or a replay: held carries the stored result
    }
    if err := applyCommand(ctx); err != nil {
        return nil, err
    }
    return nil, keeper.Complete(ctx, key, result, held)
})
```
