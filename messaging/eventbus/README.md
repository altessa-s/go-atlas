# eventbus

```go
import "github.com/altessa-s/go-atlas/messaging/eventbus"
```

Synchronous, lock-free, in-process event bus for decoupling domains that must coordinate without taking a direct dependency on one another. Handlers run
in the caller's goroutine, context, and transaction; any handler error stops the chain and propagates out of `Publish`, so a subscriber can veto the
operation that produced the event (e.g. abort the enclosing DB transaction).

## Delivery semantics

For a given event the bus runs, in registration order: type-specific handlers, then type-specific adapters, then global adapters. Dispatch deliberately
does **not** recover panics — a panicking handler must propagate so an enclosing transaction rolls back. Re-entrant publishing of an in-flight event
type fails with `ErrEventCycle`; unbounded nesting fails with `ErrDispatchTooDeep`. A type is in flight only until its dispatch returns: a handler
that keeps its context (stores it, or hands it to a goroutine) may publish the same type with it afterwards, and gets `ErrEventCycle` only while
that dispatch is still running. A nil event is checked like any type: a global adapter that publishes nil again is a cycle.

For asynchronous work a handler starts and does not wait for, publish with `Detach(ctx)`: it keeps the context's values, deadline and transaction
but starts a new dispatch chain, so a follow-up running alongside the handler is not mistaken for a re-entrant publish. The detached chain is not
checked against the one it left — a publish that is in fact nested loses cycle and depth detection — so do not use it in synchronous code.

## Key types

| Symbol | Description |
|--------|-------------|
| `Bus` | In-process bus contract (DI/testing); `InProcessBus` is the implementation |
| `New(opts ...Option) *InProcessBus` | Constructs an empty bus; `WithTxProbe` adds transaction-awareness |
| `Publisher` | Minimal producer surface (`Publish` only) |
| `Handler` / `TypedHandler[E]` | Untyped / typed event handler |
| `Detach(ctx)` | Context without the dispatch chain, for asynchronous follow-ups started by a handler |
| `TxProbe` / `AnyInTx` | Storage-agnostic "is ctx in a transaction" probe; compose several with `AnyInTx` |
| `NewObserved(bus, collector)` | Decorator recording publish latency/outcome per event type and flagging required-but-unhandled events |

## Typed helpers

`Subscribe[E]`, `RegisterAdapter[E]`, `Publish[E]`, `Require[E]` and `PublishTx[E]` are a type-safe facade over the untyped `Bus`: they derive the event
key from the static type parameter, so handlers receive `E` without a manual type assertion. `E` must be a concrete (non-interface) type.

## Usage

```go
bus := eventbus.New(eventbus.WithTxProbe(mongoProbe))

eventbus.Subscribe(bus, func(ctx context.Context, e FileUploaded) error {
    return index.Add(ctx, e.ID) // runs in the publisher's transaction
})
eventbus.Require[FileUploaded](bus)       // a missing handler fails Validate
if err := bus.Validate(); err != nil {     // run once after wiring (e.g. fx OnStart)
    return err
}

// Transactional command: dispatch only inside an active transaction.
if err := eventbus.PublishTx(ctx, bus, FileUploaded{ID: id}); err != nil {
    return err // includes ErrNotInTransaction when ctx has no transaction
}
```

## Boundaries

The bus carries commands, cascades, vetoes and notifications — interactions whose only reply is success or failure. It is **not** request/response:
`Publish` returns only an error, so a subscriber cannot hand a value back. When a caller needs a value from another domain, depend on a narrow interface
of that domain instead; the notification then flows one way through the event and the query the other way through the interface, keeping the dependency
graph acyclic. See `doc.go` for the full Locking and Boundaries rules.
