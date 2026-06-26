# uow

```go
import "github.com/altessa-s/go-atlas/domain/eventbus/uow"
```

In-process unit of work: pairs a database transaction with non-transactional side effects (S3/blobstore, cache, external calls) via
**post-commit compensation** — an in-process saga. It is the companion to [`eventbus`](../README.md)'s synchronous-in-transaction
delivery: the handler runs inside the transaction, `uow` handles the effects the handler cannot roll back. Storage-agnostic — the core
imports no driver and no `eventbus`; the transaction is supplied through the consumer-defined [`Committer`](./uow.go) interface.

## Model

1. Transactional work runs inside the transaction through a `Committer`.
2. Non-transactional `Effect`s are registered with `OnCommit` and applied **exactly once after** a successful commit.
3. If a later effect fails, the already-applied ones are compensated in **LIFO** order.

Applying after the commit means a driver-retried transaction never duplicates an effect and an aborted transaction never runs them
(clean rollback, no compensation). Compensation covers the one remaining case — an effect that fails after the commit.

## Key types

| Symbol | Description |
|--------|-------------|
| `Runner` | Executes a transactional body and applies/compensates its post-commit effects |
| `New(committer Committer, logger *slog.Logger) *Runner` | Constructs a Runner; nil logger falls back to `slog.Default` |
| `Committer` | `WithTransaction(ctx, fn)`; satisfied by `data/mongo.Mongo` |
| `Effect` | `Label`, `Apply` (post-commit), `Compensate` (nil = best-effort / irreversible) |
| `OnCommit(ctx, Effect) error` | Registers an effect from within a `Run` body; `ErrNoUnitOfWork` if none active |

## Cancellation

`Effect.Apply` runs on the caller's context (honors cancellation). `Effect.Compensate` runs on a `context.WithoutCancel` copy, so an
undo is never skipped because the caller's context was canceled.

## Usage

```go
runner := uow.New(mongo, logger) // mongo satisfies Committer

err := runner.Run(ctx, func(txCtx context.Context) error {
    if err := repo.Save(txCtx, doc); err != nil { // inside the transaction
        return err
    }
    // Defer the non-transactional put until after the commit; undo on later failure.
    return uow.OnCommit(txCtx, uow.Effect{
        Label:      "blob put",
        Apply:      func(c context.Context) error { return blob.Put(c, key, data) },
        Compensate: func(c context.Context) error { return blob.Delete(c, key) },
    })
})
```

## Durability caveat

No outbox or durable relay: a crash between the commit and applying the effects leaves the database and the external store
inconsistent — a deliberate single-process trade-off. For durable, cross-service rollback use [`data/saga`](../../../data/saga) instead.
