# Coordination & Consistency

go-atlas ships four mechanisms for keeping a state change consistent with whatever else has to happen alongside it: another domain reacting, a
non-transactional side effect, a message to another service, or a multi-step workflow. They are not interchangeable. Each sits at a different point on
two axes:

- **Boundary.** Does the work stay **in-process** (one goroutine, one transaction), or cross a **process / service** boundary (network, another
  database)?
- **Durability.** Is the coordination **ephemeral** (lost on crash) or **persisted** (survives a crash, recovers / resumes)?

| Mechanism | Boundary | Delivery | Durable | Atomicity scope | Rollback model |
|-----------|----------|----------|---------|-----------------|----------------|
| [`domain/eventbus`](domain/eventbus.md) | In-process | Synchronous | No | The caller's single transaction | Handler error → that transaction aborts |
| [`domain/eventbus/uow`](../domain/eventbus/uow/README.md) | In-process | Synchronous | No | One transaction + post-commit effects | LIFO compensation of applied effects |
| [`data/outbox`](../data/outbox/README.md) | Cross-process | Async, at-least-once | Yes | The producing transaction only | No rollback — reliable forward delivery |
| [`transport/broker`](../transport/broker/README.md) | Cross-process | Async pub/sub | Yes (with outbox) | None (it moves messages) | None — consumers must be idempotent |
| [`data/saga`](data/saga.md) | Cross-service | Step sequence | Yes (persisted) | Each step's own transaction | Reverse-order compensation, crash recovery |

---

## The shared problem: the dual write

Almost every choice here comes down to one hazard. You change state in a database **and** you need a second thing to happen: notify another domain, put
an object in a store, send a message to another service. If that second thing is **not** in the same database transaction, a crash or abort between the
two leaves the system inconsistent: the row is written but the message never sent, or the message is sent but the row rolled back.

The four mechanisms answer that one hazard differently, and which you reach for depends on where the second thing lives.

## The mechanisms

### `domain/eventbus` — decouple domains in one process

A synchronous, in-process event bus. A handler runs in the **publisher's goroutine, context, and transaction**, so its writes join the same transaction
and a handler error rolls the whole thing back (the bus's veto power). Use it to let one domain react to another's change without a direct code
dependency, and to fail the originating operation if the reaction can't proceed. No durability, no network. See the
[Event Bus guide](domain/eventbus.md).

### `domain/eventbus/uow` — non-transactional effects with compensation

The companion unit of work. When a handler needs a **non-transactional** side effect (an object-storage put, a cache write, an external call), it does
not run it inline; it registers it with `OnCommit`, and `uow` applies it **once after the commit**, compensating already-applied effects in LIFO order
if a later one fails. Closes the dual-write gap **inside one process** without an outbox. It is **not** crash-safe: a crash between the commit and the
effect leaves inconsistency. That is the deliberate single-process trade-off.

### `data/outbox` — reliable delivery out of a transaction

The transactional-outbox pattern. The event is **persisted to a store in the same transaction as the state change**, then dispatched later by background
cycles (dispatch, unlock, expire, cleanup, stats). This is the crash-safe answer to the dual write: because the event is written atomically with
the state, it cannot be lost; because it is delivered out-of-band, the transaction never waits on the network. Delivery is **at-least-once**,
transport-agnostic (the `Handler` decides how to deliver: broker, HTTP, gRPC). Lease and retention windows are evaluated against the **database
server clock** (MongoDB `$$NOW`), not the worker's wall clock, so a clock-skewed worker can neither prematurely steal a locked event nor leak one;
timestamps are stored as BSON `Date` (run `MigrateTimestampsToDate` once on a legacy collection).

Four properties are worth knowing before you design against it:

- **One attempt per cycle, with a durable backoff.** A failed event records its own retry deadline, computed with exponential backoff and jitter and
  anchored to the database clock — so it survives a restart and cannot be pulled forward by an eager instance. Retrying in-process instead would hold
  the event's lease for the whole loop and let the unlock cycle hand it to a second worker mid-flight.
- **Two terminal failure states, and neither is cleaned up.** An event that exhausts `RetryMaxAttempts` becomes `max-attempt-reached`; one whose error
  the `ShouldRetry` predicate calls permanent becomes `rejected` on the first attempt, because repeating a malformed payload cannot fix it. Together
  they are the dead-letter queue: retention sweeps the successes and leaves these for an operator.
- **Ordering is not preserved.** A batch is fetched oldest-first but dispatched concurrently, and a failed event is rescheduled behind events created
  after it. Do not assume order, even within one key.
- **The backlog is only visible if you schedule the stats cycle.** Counters cannot distinguish a stalled outbox from an idle one — both report zero.
  `outbox_events_pending`, `outbox_events_dead_lettered`, and `outbox_events_oldest_pending_age_seconds` are what an alert can be written against; see
  [metrics.md](metrics.md#outbox).

### `transport/broker` — asynchronous messaging between services

A message-broker abstraction (NATS JetStream provider) for async pub/sub across processes and services. It is the **transport** an outbox delivers to.
`transport/broker/outbox` is the broker-specific adapter over `data/outbox` (`msg.Message` conversion + broker retry), implementing `broker.Outboxer`.
Use the broker for fire-and-forget or event-driven integration where producer and consumer are decoupled in time and space. Consumers must be
**idempotent** (at-least-once), so pair them with [`data/idempotency`](../data/idempotency/README.md).

### `data/saga` — orchestrated multi-step distributed transaction

A durable, persisted orchestration saga for a business transaction that spans **several steps, each its own local transaction, possibly in different
services**. Each step has an optional compensating action; on failure the completed steps roll back in reverse order. A checkpoint is persisted after
every stage, so an instance survives a crash and can be **resumed or automatically rolled back**. The `pivot` marks the point of no return: failures
past it roll forward (retry) rather than compensate. See the [Saga guide](data/saga.md).

## Choosing

Walk the questions in order; the first match wins.

1. **Does the reaction need to run inside the same transaction and be able to veto it?** → `eventbus` (in-process, synchronous). If that reaction also
   has a non-transactional effect to undo on later failure, add `uow`.
2. **Do you need a second thing to reliably happen in another process/service after this transaction commits?** → persist it with `data/outbox` and
   deliver via `transport/broker`. Never publish to the broker directly inside the transaction; that is the dual write.
3. **Is it a multi-step business transaction across services, where each step must be compensable and the whole thing must survive a crash?** →
   `data/saga`.
4. **Is it just async pub/sub with no transactional coupling to a local write?** → `transport/broker` alone.

## How they compose

They are layers, not alternatives. A common reliable-event pipeline uses three of them together:

1. Inside the database transaction, the domain writes its state and **publishes a domain event on `eventbus`**; in-process handlers react synchronously
   (and may veto).
2. An `eventbus` **adapter persists an outbox record** in the same transaction (`data/outbox`). The cross-service hop is now crash-safe.
3. After commit, the outbox worker dispatches the record through `transport/broker` to the message broker; the remote service consumes it
   **idempotently**.

`uow` slots in for the **local** non-transactional effects that should not become messages (an object-store write you want to compensate, not relay).
`data/saga` sits **above** all of this when the operation is a multi-service workflow: the broker/outbox move its messages, while the saga owns the step
sequence, compensation, and crash recovery.

## Rules and anti-patterns

- **Never dual-write.** Writing the database and then publishing to the broker as two separate steps loses messages on a crash in between. Persist via
  `data/outbox` in the transaction; deliver after commit.
- **`eventbus` is in-process only.** Do not use it to reach another process or service — it has no network and no durability. Use the broker for that.
- **`uow` is not durability.** Its compensation handles an effect that fails *after* a successful commit; it does **not** survive a crash between commit
  and effect. When you need crash-safety, route through the outbox instead.
- **At-least-once means idempotent consumers.** Both `data/outbox` and `transport/broker` can deliver a message more than once, so consumers must
  dedupe — use [`data/idempotency`](../data/idempotency/README.md). Broker-side deduplication narrows the window but does not close it: the
  `transport/broker/outbox` adapter sends the outbox event ID as `Nats-Msg-Id` so JetStream collapses a republished event within its duplicate
  window, and a repeat that arrives after that window still reaches the consumer.
- **A dead-letter queue nobody watches is data loss with extra steps.** Terminal failures are retained deliberately and never expire on their own.
  Schedule the stats cycle and alert on `outbox_events_dead_lettered` and `outbox_events_oldest_pending_age_seconds`, or the retention is just storage.
- **Don't reach for a saga for a single local transaction.** A saga is for multi-step, multi-transaction (often multi-service) workflows with
  compensation and recovery. One local transaction with `eventbus` / `uow` is simpler and atomic; a saga there is overkill.
- **Keep the boundary honest.** In-process coordination (`eventbus` / `uow`) gives true atomicity within one transaction. Cross-process coordination
  (`outbox` / `broker` / `saga`) gives eventual consistency with compensation, never cross-database atomicity. Choose the mechanism that matches the
  boundary, not the one that's convenient.

## See also

- [Event Bus](domain/eventbus.md) — in-process delivery semantics and the `uow` companion
- [Saga](data/saga.md) — orchestration model, status lifecycle, crash recovery, examples
- Package references: [`transport/broker`](../transport/broker/README.md), [`data/outbox`](../data/outbox/README.md),
  [`data/saga`](../data/saga/README.md), [`data/idempotency`](../data/idempotency/README.md)
