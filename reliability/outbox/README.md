# outbox

```go
import "github.com/altessa-s/go-atlas/reliability/outbox"
```

Package `outbox` implements the Transactional Outbox pattern for at-least-once event delivery. Events are persisted to a `Storage` before being
dispatched via a `Handler`, with background cycles for dispatch, retry, expiration, cleanup, stuck-event recovery, and backlog measurement.
Transport-agnostic: the `Handler` callback determines delivery method (message broker, HTTP, gRPC, etc.).

## Key types

| Type / Interface | Description                                              |
|------------------|----------------------------------------------------------|
| `Outbox`         | Main outbox manager with background workers              |
| `Event`          | Stored event with delivery tracking                      |
| `Status`         | Lifecycle — see the status table below                   |
| `Handler`        | Callback that dispatches events                          |
| `Storage`          | Persistence interface for event storage                  |
| `Watcher`        | Optional `Storage` capability: push notifications on save  |
| `BackendClockSaver` | Optional `Storage` capability: stamp creation time and default expiry from the store's clock |
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
| `WithMeter`                   | no-op      | Metrics collector                                            |
| `WithLogger`                  | discard    | Structured logger                                            |
| `WithScheduler`               | nil        | Task registrar that runs the cycles                          |
| `With*Schedule`               | --         | Cron schedule per cycle (`Dispatch`, `Unlock`, `Expire`, `Cleanup`, `Stats`); empty leaves the cycle unscheduled |
| `With*TaskID`                 | `outbox-<cycle>` | Scheduler task ID per cycle; must be distinct across outboxes sharing a scheduler |

`maxLockTime` must exceed `handleTimeout`; a smaller value is raised at construction with a warning. A lock that expires while its dispatch cycle is
still publishing lets the unlock cycle hand the event to a second worker, which makes duplicate delivery the steady state rather than an edge case.

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

The whole batch is validated before anything is written, so a rejected `Save` (`ErrEmptyKey`, `ErrPayloadTooLarge`) leaves the caller's transaction
clean and abortable.

## Delivery semantics

Direct storage claims require a positive batch size. SQL returns the effective ordering key. MongoDB stores the topic only and returns an empty
`OrderKey`.

At-least-once. Duplicates are expected — a handler can publish successfully and then fail before its status is written back — so consumers must be
idempotent. Where the transport supports deduplication, key it off `Event.Id`: it is assigned once at save time and never changes across retries. The
`messaging/broker/outbox` adapter does this automatically.

Dispatch is concurrent by default. With `sqldb.WithKeyOrder()` on every SQL dispatcher, only the earliest committed, undelivered sequence for each
event `OrderKey` (or `Key` when empty) can be claimed. A failed event blocks its key during backoff. Any terminal status, including rejection or exhausted attempts,
releases the next event. Other keys can dispatch concurrently. Serialize writes for a key when dispatch must follow application revision order.

With compaction enabled, the latest `Event.CreatedAt` wins for each key within the claimed batch. Broker events use their outbox save time.

## Retries and dead-lettering

Compaction and dispatch share a budget capped at `WithHandleTimeout` or the caller deadline, whichever comes first. Failed attempts, including handler panics and handling
timeouts, count toward the retry budget. The store anchors retry backoff to its own clock. The `WithShouldRetry` predicate can send a permanent failure
straight to `rejected`; an event that exhausts its budget moves to `max-attempt-reached`.

The dispatcher records confirmed deliveries under a separate storage timeout, even during shutdown. It releases unstarted claims with their attempt history intact and preserves the
attempt count for handlers interrupted by the caller's cancellation or by a downstream outage reported with `ErrDispatchInterrupted`.

## Observability

Schedule the stats cycle (`WithStatsSchedule`) and alert on the gauges it publishes — `outbox_events_pending`, `outbox_events_dead_lettered`, and
`outbox_events_oldest_pending_age_seconds`. Counters alone cannot distinguish a stalled outbox from an idle one: both report zero. See
[docs/metrics.md](../../docs/metrics.md#outbox), which also lists the metrics renamed in this change.

## Cycles

| Cycle      | Method              | Purpose                                                      |
|------------|---------------------|--------------------------------------------------------------|
| dispatch   | `RunDispatchCycle`  | Fetch a batch, dispatch it, write back the outcome           |
| unlock     | `RunUnlockCycle`    | Reclaim events whose lock outlived `maxLockTime`             |
| expire     | `RunExpireCycle`    | Mark events past `ExpiresAt` as expired                      |
| cleanup    | `RunCleanupCycle`   | Delete processed events older than the retention window      |
| stats      | `RunStatsCycle`     | Refresh the backlog, dead-letter, and lag gauges             |

Configure cycles with `WithScheduler` and the matching `With*Schedule`, then call `RegisterTasks(ctx)`. The expire cycle is registered only with a
positive `WithDefaultEventTTL`, and the cleanup cycle only with a positive `WithPublishedEventsLifetime`. After successful registration the manual
`Run*` method returns `cycle.ErrManaged`. Task IDs are overridable and must be distinct — collisions are rejected by `RegisterTasks`
with `ErrTaskIDCollision`.

## Change notifications

A saved event otherwise waits up to one dispatch interval. When the store implements the optional `Watcher` interface, `Outbox.Watch` blocks on its
notifications and runs a dispatch cycle as soon as events land — the MongoDB store implements it with a change stream over the outbox collection, and
the [SQL store](storages/sqldb#watch) with PostgreSQL `LISTEN`/`NOTIFY` when given a listener, so an event becomes visible the instant its transaction
commits. The [factory](../../factory/reliability/outbox) starts the watcher for `outbox.watch: changeStream` or `notify`; by hand:

```go
ob.StartWatch(ctx)
defer ob.StopWatch() // Cancels the watch and waits for the storage watcher to return.
```

**Keep the dispatch schedule configured.** A notification means "something arrived", never "this event is due": a retry backoff elapsing and the unlock
cycle freeing a stuck lease are time-driven and produce no notification at all, and a notification arriving while a cycle is already running is dropped
rather than queued. `Watch` shortens the common case; the poll cycle remains the guarantee. A cycle that comes back with a full batch re-arms itself, so
a bulk insert drains without waiting for the next tick. A full cycle that advanced nothing — every event failed and was rescheduled — waits before the
next one, 100ms growing to 5s and back to none as soon as an event is sent, skipped or dead-lettered, so a handler failing fast over a large backlog
does not spin against the store. Shutdown interrupts the wait.

`ErrWatchUnsupported` is a capability report, not a failure — it is what a store without the interface, a standalone mongod with no oplog, or a SQL
store without a listener returns.
`outbox_watch_notifications_total` going flat while `outbox_events_saved_total` climbs means the watcher died and dispatch silently fell back to the
poll interval.

## Server-clock leases

The retry, lock-expiry, and retention windows passed to the `Storage` are **durations**, not absolute timestamps: `Event.RetryAfter`,
`UnlockStuckEvents(lockExpiry)`, `DeleteProcessedEvents(olderThan)`, and `ExpireEvents`. The store evaluates them against its own database server clock,
so a worker whose wall clock is skewed cannot prematurely unlock another worker's in-flight event or leak a stuck one. Writes are additionally fenced by
`Event.LockToken`, so a dispatcher that lost its lease cannot overwrite the result of the worker that took over. See [storages/mongo](./storages/mongo)
for the MongoDB `$$NOW` implementation and the one-time BSON `Date` migration, and [storages/sqldb](./storages/sqldb) for the SQL `now()` /
`UTC_TIMESTAMP(6)` implementation.

`Event.CreatedAt` and the `WithDefaultEventTTL` expiry are stamped by `Save` from the local clock, unless the store implements the optional
`BackendClockSaver` interface: then `Save` passes the instants the caller set, zero when unset, and the TTL as a duration, and the store stamps the rest
from its own clock. The SQL store does; the MongoDB store does not, because a plain insert cannot read `$$NOW`. An explicit `CreatedAt` or `ExpiresAt`
is stored as given by every store.

## Subpackages

| Package                              | Description                                                 |
|--------------------------------------|-------------------------------------------------------------|
| [factory](../../factory/reliability/outbox) | Configuration-based creation                                |
| [storages/mongo](./storages/mongo)   | MongoDB-backed event storage                                |
| [storages/sqldb](./storages/sqldb)   | PostgreSQL / MySQL / MariaDB storage through `database/sql` |
| [storages/storagetest](./storages/storagetest) | Contract suite every `Storage` backend runs           |

## Registration failures

`New` does not register schedules. Call `RegisterTasks(ctx)` explicitly and handle errors before serving traffic. If only some tasks were registered,
their callbacks return `ErrRegistrationIncomplete` until registration completes. Retry on the same outbox: successful registrations are retained and
only missing tasks are retried. Unregistered cycles remain callable manually. Repeated successful registration is a no-op.

Factories perform registration automatically. On a registration error they return both the outbox and the error, so the caller can retain that instance
and retry `RegisterTasks(ctx)`. Treat this as a failed startup until the retry succeeds.
