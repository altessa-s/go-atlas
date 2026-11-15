# outbox

```go
import "github.com/altessa-s/go-atlas/messaging/broker/outbox"
```

Package `outbox` provides a broker-specific adapter for the generic transactional outbox pattern. Wraps `reliability/outbox` with `msg.Message` conversion and
the broker retry classification, implementing the `broker.Outboxer` interface. It does not import a broker driver.

## Key types

| Type              | Description                                                                             |
|-------------------|------------------------------------------------------------------------------------------|
| `Outbox`          | Broker-specific adapter wrapping the generic `reliability/outbox.Outbox`                       |
| `Publisher`       | Interface for message transmission to external brokers                                  |
| `reliability/outbox.Storage` | Storage interface from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `reliability/outbox.Event` | Event type from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `reliability/outbox.Status` | Status type from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `reliability/outbox.Stats` | Statistics snapshot from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `ErrUndeliverable`| Marks a permanently undeliverable event so the outbox dead-letters it without retrying  |

## Deduplication

Outbox delivery is at-least-once, so the broker needs a stable identity to collapse repeats. On publish this adapter sets the `deduplicate_id` metadata
key to the outbox `Event.Id` — assigned once at save time and unchanged across every retry — which the NATS provider forwards as `Nats-Msg-Id`. A
caller-supplied `deduplicate_id` always wins: it is usually derived from the business entity, which dedupes across producers and not only across one
event's own retries.

## Error classification

A stored payload this adapter cannot decode is wrapped in `ErrUndeliverable`, and the retry predicate reports it as permanent so the event goes straight
to `rejected` instead of consuming its whole retry budget. The predicate also reports `broker.ErrConnectionClosed` as permanent, so an event whose
publish fails on a closed connection is rejected as well; every other error stays transient and is retried with backoff. Providers mark their driver's
closed-connection error with that sentinel (the NATS provider does); a custom `Publisher` that returns a raw driver error such as
`nats.ErrConnectionClosed` must wrap it with `broker.ErrConnectionClosed` to keep it permanent.

## Methods

| Method        | Description                                                                                                       |
|---------------|-------------------------------------------------------------------------------------------------------------------|
| `New`         | Create a new broker outbox adapter with a `outbox.Storage`, `Publisher`, and `reliability/outbox` options (`...outbox.Option`)  |
| `NewHandler`  | The `outbox.Handler` that decodes a stored broker envelope and publishes it as a `msg.Message`                    |
| `ShouldRetry` | The broker retry predicate for `outbox.WithShouldRetry` (see [Error classification](#error-classification))       |
| `Publish`     | Save a single message to the outbox store for later publishing                                                    |
| `PublishBatch`| Save multiple messages to the outbox store in a single operation                                                  |

The adapter also exposes these methods inherited from `reliability/outbox.Outbox`:

| Method | Description |
|--------|-------------|
| `RegisterTasks(ctx)` | Register configured background schedules |
| `RunDispatchCycle(ctx)` | Fetch and dispatch pending events once |
| `RunUnlockCycle(ctx)` | Release stale event locks once |
| `RunCleanupCycle(ctx)` | Delete old published events once |
| `RunExpireCycle(ctx)` | Expire overdue events once |
| `RunStatsCycle(ctx)` | Refresh backlog, dead-letter, and lag gauges once |
| `RegisterDispatchSchedulerFunc`, `RegisterUnlockSchedulerFunc`, `RegisterCleanupSchedulerFunc`, `RegisterExpireSchedulerFunc`, `RegisterStatsSchedulerFunc` | Return scheduler callbacks and mark the corresponding cycles scheduler-managed |
| `Watch(ctx)` | Dispatch on storage notifications; requires watcher support and supplements the dispatch schedule |

## Options

`New` takes the [`reliability/outbox`](../../../reliability/outbox) options directly (`outbox.WithLogger`, `outbox.WithHandleTimeout`,
`outbox.WithEventsBatchSize`, `outbox.WithCompaction`, `outbox.WithScheduler`, the `With*Schedule` and `With*TaskID` options, and so on) — see its
README for the full table. The adapter appends `outbox.WithShouldRetry(ShouldRetry)` after them, so a caller-supplied retry predicate is overridden.

```go
import dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"

ob := outbox.New(store, publisher,
    dataoutbox.WithLogger(logger),
    dataoutbox.WithHandleTimeout(5*time.Second),
    dataoutbox.WithMaxLockTime(time.Minute),
)
```

`WithMaxLockTime` must exceed `WithHandleTimeout`; a smaller value is raised at construction with a warning. Without `WithMeter` and execution of
the stats cycle (through `WithStatsSchedule`, a registered callback, or `RunStatsCycle`), the backlog, dead-letter, and lag gauges are not refreshed,
which is indistinguishable from a healthy idle outbox — see [docs/metrics.md](../../../docs/metrics.md#outbox).

## Scheduler registration

The adapter inherits `RegisterTasks(ctx)` from `reliability/outbox`. Its constructor does not install schedules; call this method after configuring
`WithScheduler` and handle errors. Partial registration gates callbacks until a retry on the same instance succeeds.
[Factory](../../../factory/messaging/broker) constructors register automatically and retain the outbox alongside any registration error.
