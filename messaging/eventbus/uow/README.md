# uow

```go
import "github.com/altessa-s/go-atlas/messaging/eventbus/uow"
```

In-process unit of work: pairs a database transaction with non-transactional side effects (S3/blobstore, cache, external calls) via **post-commit
compensation** — an in-process saga. It is the companion to [`eventbus`](../README.md)'s synchronous-in-transaction delivery: the handler runs inside
the transaction, `uow` handles the effects the handler cannot roll back. Storage-agnostic — the core imports no driver and no `eventbus`; the
transaction is supplied through the consumer-defined [`Committer`](./uow.go) interface. The test double lives in [`uowtest`](./uowtest/README.md).

## Model

1. Transactional work runs inside the transaction through a `Committer`.
2. Non-transactional `Effect`s are registered with `OnCommit` and applied **exactly once after** a successful commit.
3. If a later effect fails, the already-applied ones are compensated in **LIFO** order.
4. Fire-and-forget hooks (cache invalidation) are registered with `Runner.AfterCommit` and run after the commit, in registration order with the
   effects; a hook cannot fail the transaction and its panic is recovered and logged.

A panic in an effect is handled as an effect failure. Compensation continues
after a compensator panics, and subsequent hooks still run.

Applying after the commit means a driver-retried transaction never duplicates an effect and an aborted transaction never runs them (clean rollback, no
compensation). Compensation covers the one remaining case — an effect that fails after the commit.

## Key types

| Symbol                                                                  | Description                                                                                           |
|-------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------|
| `Runner`                                                                | Executes a transactional body and applies its effects and hooks; safe for concurrent use               |
| `New(committer Committer, logger *slog.Logger, opts ...Option) *Runner` | Constructs a Runner; nil logger falls back to `slog.Default`                                          |
| `Runner.Run(ctx, body) error`                                           | Root: one transaction; nested (ctx carries an open unit of the Runner): joins it                      |
| `Runner.AfterCommit(ctx, hook) error`                                   | Registers a hook on the open unit; outside one, runs it at once or returns `ErrNoUnitOfWork`          |
| `OnCommit(ctx, Effect) error`                                           | Registers an effect on the innermost unit; `ErrNoUnitOfWork` if none, `ErrScopeClosed` if it ended   |
| `Committer`                                                             | `WithTransaction(ctx, fn)`; implemented by `store/mongo.Mongo` and `store/postgres.Committer`                                           |
| `OutcomeClassifier`                                                     | Optional committer method that distinguishes an ambiguous commit error from a definitive abort         |
| `OutcomeCommitter`                                                      | Explicit transaction API for callers that need a commit flag beside the returned error                |
| `Detector`                                                              | `func(ctx) (tx any, ok bool)`: the innermost running transaction of any client and its identity; `store/mongo.Transaction`, `store/postgres.Transaction` and `(*postgres.Committer).Detect` have this shape |
| `TransactionDetector`                                                   | Optional committer method supplying the default detector. Implemented by the MongoDB and PostgreSQL committers and `uowtest.Fake` |
| `Effect`                                                                | `Label`, `Apply` (post-commit), `Compensate` (nil = best-effort / irreversible)                      |
| `UnknownOutcomeError`                                                   | The body returned nil but the commit did not report success; `Err` is the Committer error             |

## Errors

| Sentinel                | Returned when                                                                                                     |
|-------------------------|-------------------------------------------------------------------------------------------------------------------|
| `ErrNoUnitOfWork`       | `OnCommit`, or `AfterCommit` without `WithRunHooksOutsideTransaction`, is called outside a `Run` body             |
| `ErrForeignTransaction` | ctx carries a detected transaction that the Runner did not start, or an open unit of another Runner |
| `ErrScopeClosed`        | ctx carries a unit that ended: retained after `Run` returned, or of an attempt the driver aborted and retried   |
| `ErrUnknownOutcome`     | Matched by `*UnknownOutcomeError`; the writes may or may not have committed                                       |

## Semantics

| Situation                                        | Behavior                                                                                                    |
|--------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| Driver retries the callback                      | Each attempt gets a fresh unit; the aborted attempt's unit is closed and its registrations dropped          |
| Commit-only retry                                | The callback is not re-run; its unit is kept                                                                |
| Body returns an error                            | The Committer error is returned as is; nothing is applied                                                   |
| Body panics, the Committer recovers it           | Known abort (`store/mongo.ErrTransactionPanicked`, `store/postgres.ErrTransactionPanicked`): returned as is, never `ErrUnknownOutcome`  |
| Body panics, the Committer propagates it         | The unit is closed and the panic propagates                                                                 |
| Body returns nil, the Committer fails            | `*UnknownOutcomeError` unless the committer reports a definitive abort; nothing is applied                                                                  |
| Commit succeeds and an observer fails            | The adapter confirms the transaction that ran the body through `WithCommitConfirmation`. The Runner applies its effects and hooks and returns any observer and effect errors |
| Nested `Run` of the same Runner                  | Calls the body directly; registrations join the root unit and apply after the outermost commit            |
| PostgreSQL savepoint                             | Hooks and effects apply after root commit only if every enclosing savepoint was released. A retained savepoint context returns `ErrScopeClosed`. |
| Hook after the root commit                       | Runs on `context.WithoutCancel(ctx)`; runs even when an earlier effect failed; a panic is recovered       |

With a detector, a successful root body must leave its captured transaction running. If it ended (including an explicit abort) or changed
identity (`store/mongo.Transaction` also tells apart a transaction restarted on the same session), `Run` rejects the attempt with
`ErrForeignTransaction`, discards its registrations, and does not report `ErrUnknownOutcome`. Without a detector, the `Committer` must report an
explicit abort as an error. Bodies must leave transaction completion to the `Committer`. A panic in a joined `Run` closes the unit. If a caller
recovers inside the root body and returns nil, the root rolls back with `ErrScopeClosed`.

The Runner calls `WithTransaction` through the supplied Committer. Wrappers can
route or reject the call and should preserve the supplied context and error
chain when delegating.

## Cancellation

`Effect.Apply` runs on the caller's context (honors cancellation). `Effect.Compensate` runs on a `context.WithoutCancel` copy, so a caller cancellation
does not skip the undo. The entire compensation pass has a deadline (default 30s); callbacks must honor it. Hooks run on a `context.WithoutCancel`
copy: a commit has happened, so a caller that walks away afterwards does not lose the invalidation.

## Options

| Option                           | Default | Effect                                                                                               |
|----------------------------------|---------|------------------------------------------------------------------------------------------------------|
| `WithCompensationTimeout`        | 30s     | Overall compensation budget, independent of the original caller deadline                            |
| `WithDetector`                   | Committer's `DetectTransaction`, if provided | Overrides detection of active transactions and their identity |
| `WithRunHooksOutsideTransaction` | off     | `AfterCommit` outside a unit of work and a transaction runs the hook at once instead of failing     |

## Usage

```go
runner := uow.New(mongo, logger) // mongo satisfies Committer and TransactionDetector

err := runner.Run(ctx, func(txCtx context.Context) error {
    if err := repo.Save(txCtx, doc); err != nil { // inside the transaction
        return err
    }
    // Invalidate the cache only after the commit, even if ctx is canceled meanwhile.
    if err := runner.AfterCommit(txCtx, func(c context.Context) { cache.Invalidate(c, doc.ID) }); err != nil {
        return err
    }
    // Defer the non-transactional put until after the commit; undo on later failure.
    return uow.OnCommit(txCtx, uow.Effect{
        Label:      "blob put",
        Apply:      func(c context.Context) error { return blob.Put(c, key, data) },
        Compensate: func(c context.Context) error { return blob.Delete(c, key) },
    })
})
if errors.Is(err, uow.ErrUnknownOutcome) {
    // The commit may have happened: re-read before retrying a non-idempotent write.
}
```

## Durability caveat

No outbox or durable relay: a crash between the commit and applying the effects leaves the database and the external store inconsistent — a deliberate
single-process trade-off. For durable, cross-service rollback use [`reliability/saga`](../../../reliability/saga) instead.
