# leadelect

```go
import "github.com/altessa-s/go-atlas/coord/leadelect"
```

Package `leadelect` provides distributed leader election for service coordination. Ensures only one instance is active as leader among multiple
replicas, with automatic re-election on node failures and a callback system for leadership transitions.

## Key types

| Type / Interface | Description                                             |
|------------------|---------------------------------------------------------|
| `Leader`         | Main leader election manager                            |
| `LeaderElector`  | Interface: LeaderId, IsLeader, Fence, NodeId, IsRunning |

## Fencing

`IsLeader` reports leadership but cannot, on its own, stop a frozen or partitioned former leader from acting on a stale belief. `Fence` closes that
gap:
it returns a monotonically non-decreasing token for the current leadership term (the NATS provider sources it from the JetStream KV revision), or `0`
when this node is not a fresh leader. Thread the token into the conditional write of a downstream store — record the highest token accepted and reject
any lower one — so a zombie leader's late write is rejected even if it still believes `IsLeader`.

## Lifecycle

`Start` spawns a dispatch goroutine that consumes leadership transitions and runs the registered callbacks synchronously, one transition at a time.
Callback delivery waits until startup publishes its state and releases the lifecycle lock, so a callback can immediately stop the election without
losing its request. `Start` returns nil when startup succeeds, even if a callback immediately stops the election.

`Stop` tears that down deterministically: it stops the provider first — so the provider's terminal notification still lands — then ends the dispatch
goroutine and joins it. Once `Stop` returns, no callback is executing and none will start. A transition that `Stop` raced may be skipped; declining to
begin new leadership work while shutting down is intended. The context given to `Stop` bounds the provider's own shutdown, including resignation of
the
lease.

Canceling the context passed to `Start` also ends the election: the provider gives up leadership before it releases the lease, and `IsRunning`,
`IsLeader` and `Fence` — all read from the provider — report false, false and `0`. A node that was leader runs its loss callbacks exactly once, as for
any other loss: the callbacks run on a context detached from the canceled one and bounded by the handler timeout, and a `Stop` or `Start` that
follows waits for them instead of repeating or dropping them.

`Start` on a running `Leader` is a no-op. `Start` after the election was ended by its context finishes that run and starts a new election, so a
caller that restarts on cancellation does not need `Stop` in between. Call `Stop` when the `Leader` is no longer needed: it joins the goroutines of a
run ended by its context too.

The loss callbacks run exactly once for every term the leader callbacks ran for, however the term ended — a lost lease, a canceled context or `Stop`
on a live leader — and never twice when two of those coincide. They run on a context that `Stop` does not cancel, bounded by the
handler timeout, and `Stop` returns after them. A loss followed by a new term reaches the callbacks in that order even when both were queued behind
a running callback.

A callback may call `Stop` on its `Leader` — to give up after a failed leader initialization, for example. That `Stop` ends the election and
returns without waiting for the callbacks, the caller among them; the loss callbacks for the term still run, and a later `Stop` or `Start` from
outside waits for them. `Start` from a callback returns `ErrStartFromCallback`: restart from another goroutine. A call is recognized as coming
from a callback by its goroutine or by a context derived from the callback's; a goroutine the callback starts must pass such a context.

If another lifecycle call holds the lock, callback `Stop` records a request for that run and returns without waiting. Even an idempotent `Start`
honors that request and resigns the provider before completing. The request cannot stop a later run.

Transitions are buffered, so a leadership change raised while a previous callback is still running is queued rather than dropped — losing a "
leadership
lost" edge is how a node keeps acting as leader after it is not. The buffers are bounded: when transitions pile up behind a slow consumer the NATS
provider coalesces them to the latest state — the consumer always ends on the current one, and intermediate edges may be skipped, but a change of
term never is: a leader that lost and regained leadership gets its loss callbacks and then its leader callbacks for the new term — so keep callbacks
short and treat `IsLeader` and `Fence` as the source of truth.

## Constructor

```go
le := leadelect.New(provider, key, nodeID, opts...)
```

`key` and `nodeID` are required positional arguments — the election key shared across all replicas, and this instance's unique identifier.
`nats.NewLeader(ctx, conn, key, nodeID, opts...)` in [providers/nats](./providers/nats) builds the same `Leader` over a NATS provider created from
`conn`; this package itself does not import a driver.

## Options

| Option               | Default | Description                              |
|----------------------|---------|------------------------------------------|
| `WithTTL`            | 10s     | Lease duration before leadership expires |
| `WithHandlerTimeout` | 3s      | Timeout for callback execution           |
| `WithMeter`          | no-op   | Metrics collector                        |

## Subpackages

| Package                                  | Description                  |
|------------------------------------------|------------------------------|
| [factory](../../factory/coord/leadelect) | Configuration-based creation |
| [providers/nats](./providers/nats)       | NATS JetStream provider      |
