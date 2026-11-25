# uowtest

```go
import "github.com/altessa-s/go-atlas/messaging/eventbus/uow/uowtest"
```

Scriptable in-memory transaction for testing code built on [`uow`](../README.md). `Fake` is a `uow.Committer` that supplies `Transaction`
as its default detector; its fields script driver retries, commit failures, lost commit replies and panic recovery, and roll back registered stores on abort.

## Key types

| Symbol        | Description                                                                                                  |
|---------------|--------------------------------------------------------------------------------------------------------------|
| `Fake`        | `uow.Committer`; a nested call inside its own innermost transaction joins it, inside another Fake's it starts its own |
| `Transaction` | `uow.Detector`: the innermost running attempt of any `Fake` in ctx and its identity                          |
| `Snapshotter` | `Snapshot() (restore func())`; implemented structurally by in-memory stores listed in `Fake.Isolate`        |
| `ErrPanicked` | Wrapped by the error `WithTransaction` returns for a recovered callback panic (`RecoverPanics`)              |

## Fake fields

| Field           | Effect                                                                                                         |
|-----------------|----------------------------------------------------------------------------------------------------------------|
| `Attempts`      | Callback attempts per transaction (at least 1); every attempt but the last is a transient abort, rolled back  |
| `CommitRetries` | Commit-only retries after the last attempt; the callback is not re-run                                        |
| `Fail`          | Returned instead of committing; the last attempt is rolled back                                               |
| `Abort`         | `func(txn int) error`; a non-nil result is returned instead of committing and the attempt is rolled back      |
| `Unknown`       | `func(txn int) error`; a non-nil result is returned after a commit that is kept (a lost commit reply)          |
| `RecoverPanics` | Recovers a callback panic, rolls back and returns an error wrapping `ErrPanicked`, as `store/mongo` does       |
| `Isolate`       | Stores snapshotted before every attempt and restored after every aborted attempt                              |

Counters: `Calls()` (callback invocations), `Txns()` (root transactions), `CommitRetriesDone()`, `CommitAttempts()` (transactions + commit
retries).

## Usage

```go
f := &uowtest.Fake{Attempts: 2, Isolate: []uowtest.Snapshotter{store}}
runner := uow.New(f, nil)

err := runner.Run(ctx, func(txCtx context.Context) error {
    return store.Save(txCtx, doc)
})
```
