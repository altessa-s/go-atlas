# Event Bus (`eventbus`)

```go
import "github.com/altessa-s/go-atlas/domain/eventbus"
```

> The package lives under `domain/eventbus/`. Its quick reference is the package [`README`](../../domain/eventbus/README.md); this guide is the
> narrative: delivery and transaction semantics, the companion [`uow`](../../domain/eventbus/uow/README.md) unit of work, and how the pair stays
> storage-agnostic across any database.

A synchronous, lock-free, in-process event bus for decoupling domains that must coordinate without taking a direct dependency on one another. Handlers
run in the caller's goroutine, context, and database transaction; any handler error stops the chain and propagates out of `Publish`, so a subscriber can
veto the operation that produced the event (for example, abort the enclosing transaction).

It is **not** a message broker. For inter-process / inter-service messaging use [`transport/broker`](../../transport/broker); for durable, persisted,
cross-service rollback use [`data/saga`](../../data/saga). `eventbus` is the in-process counterpart: one goroutine, one transaction, no network.

---

## Delivery and transaction semantics

For a given event the bus runs, **in registration order**: the type-specific handlers, then the type-specific adapters, then the global adapters. It
stops at the first error and returns it. A `nil` event matches no type-specific handlers but still runs the global adapters.

- **Synchronous, same goroutine.** A handler runs on the publisher's `context.Context`, so whatever transaction that context carries is the transaction
  the handler's writes land in. Returning an error rolls that transaction back; that is the bus's veto power.
- **No panic recovery.** Dispatch deliberately does not recover panics: a panicking handler must propagate so an enclosing transaction rolls back rather
  than committing partial work.
- **Lock-free reads.** The handler/adapter tables use atomic copy-on-write, so the hot `Publish` path takes no lock; only registration (`Subscribe`,
  `RegisterAdapter`, `RequireHandler`) serializes on a mutex.

### Re-entrancy and depth guards

Because dispatch is synchronous, the chain can cascade (a handler publishes another event). Two guards bound that:

| Error | Returned when |
|-------|---------------|
| `ErrEventCycle` | An event type is published again while it is already in flight in the same chain (e.g. handler A publishes B whose handler publishes A) |
| `ErrDispatchTooDeep` | Nested publishing exceeds the depth limit (32), guarding against unbounded recursion |

Both are matchable with `errors.Is`. They do **not** detect a deadlock on a non-event application lock. See Locking.

## Locking

Any application lock held when `Publish` is called stays held for the entire handler chain (dispatch is synchronous). Do not call `Publish` while
holding a lock that a handler might try to acquire, directly or through a call back into the publishing service; that deadlocks the goroutine. Pass the
state handlers need inside the event itself rather than letting them reach back into the publisher's internals.

## Typed API

The generic helpers are a type-safe facade over the untyped `Bus`: they derive the event key from the static type parameter `E` (which must be a
concrete, non-interface type), so a typed publish always reaches its typed subscribers and handlers receive `E` without a manual type assertion.

| Symbol | Role |
|--------|------|
| `Subscribe[E](bus, handler)` | Register a typed handler for events of type `E` |
| `RegisterAdapter[E](bus, adapter)` | Register a typed adapter (runs after all handlers for `E`) |
| `Publish[E](ctx, pub, event)` | Publish a typed event, keyed by its own type `E` |
| `Require[E](bus)` | Declare that `E` must have at least one handler (checked by `Validate`) |
| `PublishTx[E](ctx, bus, event)` | Publish only inside an active transaction; else `ErrNotInTransaction` |

## Adapters, requirements, and validation

**Handlers** carry business logic and run first. **Adapters** run after all handlers for a type and are the integration point for post-handler side
effects (for example, mirroring an event to a broker); an adapter error aborts the publish exactly like a handler error. **Global adapters** run for
every event type.

`RequireHandler` / `Require[E]` records that an event type must have a business handler by the time `Validate` runs. Call `Validate` once after wiring
(e.g. an fx `OnStart` hook) so a missing subscription fails startup instead of silently dropping events. Adapters do not satisfy a requirement: a
required event must have a real handler.

## Observability

The core bus emits no metrics. Wrap it with `NewObserved(bus, collector)` to record, per event type, publish latency and a success/error count, and to
flag the silent non-delivery of a *required* event (a `publish_no_handler_total` counter plus a warning log). A `nil` collector falls back to a no-op,
so the decorator is always safe to construct. See the [Metrics Reference](../metrics.md) for the exact metric names.

## Boundaries

The bus carries commands, cascades, vetoes, and notifications: interactions whose only reply is success or failure. It is **not** request/response:
`Publish` returns only an error, so a subscriber cannot hand a value back. Do not smuggle a result out by mutating the event; with several handlers that
races and hides the real contract. When a caller needs a value from another domain, depend on a narrow interface of that domain instead: the
notification flows one way through the event, the query the other way through the interface, and the dependency graph stays acyclic.

---

## Transactional unit of work (`uow`)

A synchronous-in-transaction handler can only roll back the transactional store: aborting the transaction undoes the database writes but not any
non-transactional effect: an object-storage put, a cache mutation, an external call. The companion [`uow`](../../domain/eventbus/uow/README.md) package
closes that gap with **post-commit compensation** (an in-process saga), without an outbox or distributed transaction.

The model:

1. Transactional work runs inside the database transaction through a `Committer` (`WithTransaction(ctx, fn) error`).
2. Non-transactional effects are **not** run inline — a handler registers them with `uow.OnCommit`, and they apply exactly once **after** a successful
   commit.
3. If a later effect fails, the already-applied ones are compensated in **LIFO** order.

Applying after the commit means a driver-retried transaction never duplicates an effect, and an aborted transaction simply never runs them (clean
rollback, no compensation). `Effect.Apply` runs on the caller's context; `Effect.Compensate` runs on a `context.WithoutCancel` copy, so an undo is never
skipped because the caller's context was canceled. A `nil` `Compensate` marks the effect best-effort / irreversible.

`uow` and `eventbus` do **not** import each other; they meet only through the `context`. `uow.Run` puts the unit of work and the transaction in `ctx`;
`WithTransaction` derives a child `ctx` that preserves both; `eventbus.Publish` passes that `ctx` to handlers (it preserves parent values); the handler
calls `uow.OnCommit(ctx, …)` and finds the unit. The bus's `TxProbe` and `uow`'s `Committer` key off the same transaction, so inside `uow.Run` the bus
reports `InTransaction == true` and `PublishTx` works.

```go
runner := uow.New(db, logger)                  // db satisfies uow.Committer
bus := eventbus.New(eventbus.WithTxProbe(dbProbe))
eventbus.Subscribe(bus, indexOnFileUploaded)

func createFile(ctx context.Context, in Input) error {
    return runner.Run(ctx, func(txCtx context.Context) error {
        if err := repo.Save(txCtx, in.Doc); err != nil { // inside the transaction
            return err                                    // abort: no effect applied
        }
        return eventbus.PublishTx(txCtx, bus, FileUploaded{ID: in.Doc.ID})
    })
}

func indexOnFileUploaded(txCtx context.Context, e FileUploaded) error {
    if err := index.Add(txCtx, e.ID); err != nil {        // transactional write
        return err
    }
    return uow.OnCommit(txCtx, uow.Effect{                 // non-tx effect, deferred
        Label:      "object put",
        Apply:      func(c context.Context) error { return store.Put(c, e.ID, data) },
        Compensate: func(c context.Context) error { return store.Delete(c, e.ID) },
    })
}
```

## Database backends

Neither `eventbus` nor `uow` imports a database driver. A database is wired in through two small consumer-defined seams; the cores never change.
Switching or adding a database means writing these two adapters where the database handle lives. Nothing in `eventbus` / `uow` is touched.

| Seam | Contract | What you supply |
|------|----------|-----------------|
| `uow.Committer` | `WithTransaction(ctx, fn) error` | A type that opens a transaction and passes a transaction-scoped context to `fn` (your database handle; `data/mongo.Mongo` already satisfies it) |
| `eventbus.TxProbe` | reports whether `ctx` carries an active transaction | A function that detects your database's transaction in `ctx` |

The one behavioral nuance is per-database, not per-design. Some drivers carry the transaction in the context and **retry** the `WithTransaction`
callback on transient errors (for example, the MongoDB driver), which is exactly why `uow` resets its registered effects on every attempt. Others have
no ambient session, so the `Committer` must **thread the transaction handle through the context explicitly** and repositories read the executor from the
context, the "transactor" pattern (for example, a SQL database via `database/sql` or pgx). `uow`'s per-attempt reset covers both, whether your committer
runs the callback once or wraps it in a retry loop.

A sketch of a `Committer` for a database with no ambient session (here, a SQL-style handle), to show the two seams concretely:

```go
type SQL struct{ pool *sql.DB }
type txKey struct{}

func (s *SQL) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
    tx, err := s.pool.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    txCtx := context.WithValue(ctx, txKey{}, tx) // derives from ctx → uow unit + probe survive
    if err := fn(txCtx); err != nil {
        _ = tx.Rollback()
        return err
    }
    return tx.Commit()
}

func InTx(ctx context.Context) bool { _, ok := ctx.Value(txKey{}).(*sql.Tx); return ok } // eventbus.TxProbe
```

Two rules any backend `Committer` must obey: derive the transaction context **from the input `ctx`** (not from `context.Background`), or the unit of
work and the probe are lost; and, when the database has no ambient session, have repositories read their executor from the context.

When several databases coexist, `eventbus.AnyInTx(probeA, probeB)` composes their probes so `InTransaction` is true if any is active. Note the
limitation: `uow` gives one transaction's atomicity plus its post-commit effects. Writes spanning **two different databases** are two separate
transactions, not one atomic unit; the second database has to be modeled as a compensated effect.
