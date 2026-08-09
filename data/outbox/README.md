# outbox

```go
import "github.com/altessa-s/go-atlas/data/outbox"
```

Package `outbox` implements the Transactional Outbox pattern for at-least-once event delivery. Events are persisted to a `Store` before being
dispatched via a `Handler`, with background cycles for dispatch, retry, expiration, cleanup, stuck-event recovery, and backlog measurement.
Transport-agnostic: the `Handler` callback determines delivery method (message broker, HTTP, gRPC, etc.).

## Key types

| Type / Interface | Description                                              |
|------------------|----------------------------------------------------------|
| `Outbox`         | Main outbox manager with background workers              |
| `Event`          | Stored event with delivery tracking                      |
| `Status`         | Lifecycle — see the status table below                   |
| `Handler`        | Callback that dispatches events                          |
| `Store`          | Persistence interface for event storage                  |
| `Stats`          | Backlog snapshot: queue depth, dead-letter depth, lag    |

## Statuses

| Status                | Terminal | Meaning                                                              |
|-----------------------|----------|----------------------------------------------------------------------|
| `pending`             | no       | Awaiting first dispatch                                              |
| `in-progress`         | no       | Locked by a dispatcher                                               |
| `failed`              | no       | Attempt failed; retried once the backoff elapses                     |
| `sent`                | yes      | Dispatched successfully                                              |
| `skipped`             | yes      | Superseded by a newer event with the same key (compaction)           |
| `expired`             | yes      | Passed its `ExpiresAt` before being dispatched                       |
| `max-attempt-reached` | yes      | Exhausted the retry budget — dead-lettered                           |
| `rejected`            | yes      | Classified as permanently undeliverable — dead-lettered              |

`max-attempt-reached` and `rejected` are the dead-letter queue: they are excluded from cleanup and stay until an operator acts on them.

## Options

| Option                        | Default    | Description                                                  |
|-------------------------------|------------|--------------------------------------------------------------|
| `WithFetchTimeout`            | 5s         | Store fetch timeout                                          |
| `WithHandleTimeout`           | 20s        | Per-cycle dispatch timeout                                   |
| `WithUpdateTimeout`           | 5s         | Store update timeout                                         |
| `WithMaxLockTime`             | 40s        | Lock lifetime before the unlock cycle reclaims an event      |
| `WithEventsBatchSize`         | 200        | Events fetched per batch                                     |
| `WithRetryMaxAttempts`        | 10         | Maximum dispatch attempts before dead-lettering              |
| `WithRetryBaseDelay`          | 1s         | Backoff before the second attempt                            |
| `WithRetryMaxDelay`           | 5m         | Backoff ceiling                                              |
| `WithMaxPayloadBytes`         | 1 MiB      | Payload size limit; 0 disables the check                     |
| `WithPublishedEventsLifetime` | indefinite | Retention for processed events                               |
| `WithDefaultEventTTL`         | off        | Default `ExpiresAt` applied at save time                     |
| `WithCompaction`              | off        | Deduplicate events by key within a batch                     |
| `WithCompactionFilter`        | nil        | Selective key-based compaction                               |
| `WithShouldRetry`             | nil        | Transient/permanent error predicate                          |
| `WithCollector`               | no-op      | Metrics collector                                            |
| `WithLogger`                  | discard    | Structured logger                                            |

`maxLockTime` must exceed `handleTimeout`; a smaller value is raised at construction with a warning. A lock that expires while its dispatch
cycle is still publishing lets the unlock cycle hand the event to a second worker, which makes duplicate delivery the steady state rather than
an edge case.

## Usage inside a transaction

`Save` only closes the dual-write gap if it runs in the same transaction as the business data:

```go
_, err := sess.WithTransaction(ctx, func(sessCtx context.Context) (any, error) {
    if err := repo.CreateOrder(sessCtx, order); err != nil {
        return nil, err
    }
    return nil, ob.Save(sessCtx, outbox.Event{Key: "billing.order.created", Payload: payload})
})
```

The whole batch is validated before anything is written, so a rejected `Save` (`ErrEmptyKey`, `ErrPayloadTooLarge`) leaves the caller's
transaction clean and abortable.

## Delivery semantics

At-least-once. Duplicates are expected — a handler can publish successfully and then fail before its status is written back — so consumers must
be idempotent. Where the transport supports deduplication, key it off `Event.Id`: it is assigned once at save time and never changes across
retries. The `transport/broker/outbox` adapter does this automatically.

**Ordering is not preserved.** Batches are fetched oldest-first but dispatched concurrently, and a failed event is rescheduled behind events
created after it. Do not assume ordering, even within a single `Key`.

## Retries and dead-lettering

Each cycle spends exactly one attempt per event; spacing comes from an exponential backoff with jitter that the store anchors to its own clock.
Errors are classified by `WithShouldRetry`: transient ones are rescheduled, permanent ones (a malformed payload, an unrejectable subject) go
straight to `rejected` rather than burning the whole budget first. Context cancellation and timeouts always count as transient, so a broker
outage cannot dead-letter healthy events.

## Observability

Schedule the stats cycle (`WithStatsSchedule`) and alert on the gauges it publishes — `outbox_events_pending`, `outbox_events_dead_lettered`,
and `outbox_events_oldest_pending_age_seconds`. Counters alone cannot distinguish a stalled outbox from an idle one: both report zero. See
[docs/metrics.md](../../docs/metrics.md#outbox), which also lists the metrics renamed in this change.

## Cycles

| Cycle      | Method              | Purpose                                                      |
|------------|---------------------|--------------------------------------------------------------|
| dispatch   | `RunDispatchCycle`  | Fetch a batch, dispatch it, write back the outcome           |
| unlock     | `RunUnlockCycle`    | Reclaim events whose lock outlived `maxLockTime`             |
| expire     | `RunExpireCycle`    | Mark events past `ExpiresAt` as expired                      |
| cleanup    | `RunCleanupCycle`   | Delete processed events older than the retention window      |
| stats      | `RunStatsCycle`     | Refresh the backlog, dead-letter, and lag gauges             |

Each cycle can be registered with a scheduler (`WithScheduler` plus the matching `With*Schedule`), after which the manual `Run*` method returns
`corescheduler.ErrSchedulerManaged`. Task IDs are overridable and must be distinct — collisions are rejected at startup with
`ErrTaskIDCollision`.

## Server-clock leases

The retry, lock-expiry, and retention windows passed to the `Store` are **durations**, not absolute timestamps: `Event.RetryAfter`,
`UnlockStuckEvents(lockExpiry)`, `DeleteProcessedEvents(olderThan)`, and `ExpireEvents`. The store evaluates them against its own database
server clock, so a worker whose wall clock is skewed cannot prematurely unlock another worker's in-flight event or leak a stuck one. Writes are
additionally fenced by `Event.LockToken`, so a dispatcher that lost its lease cannot overwrite the result of the worker that took over. See
[store/mongo](./store/mongo) for the MongoDB `$$NOW` implementation and the one-time BSON `Date` migration.

## Subpackages

| Package                          | Description                          |
|----------------------------------|--------------------------------------|
| [factory](./factory)             | Configuration-based creation         |
| [store/mongo](./store/mongo)     | MongoDB-backed event storage         |
