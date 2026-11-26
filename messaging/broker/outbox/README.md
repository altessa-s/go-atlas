# outbox

```go
import "github.com/altessa-s/go-atlas/messaging/broker/outbox"
```

Package `outbox` provides a broker-specific adapter for the generic transactional outbox pattern. Wraps `reliability/outbox` with `msg.Message` conversion and
the broker retry classification, implementing the `broker.Outboxer` interface. It does not import a broker driver.

## Key types

| Type              | Description                                                                             |
|-------------------|------------------------------------------------------------------------------------------|
| `Outbox`          | Broker-specific adapter. Use `Outbox{Saver: saver}` for publishing with a custom saver                       |
| `Saver`           | Persists one batch of events in the publisher’s context |
| `Management`      | Supplies the watch, cycle and scheduling methods of a managed outbox |
| `Publisher`       | Interface for message transmission to external brokers                                  |
| `reliability/outbox.Storage` | Storage interface from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `reliability/outbox.Event` | Event type from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `reliability/outbox.Status` | Status type from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `reliability/outbox.Stats` | Statistics snapshot from `github.com/altessa-s/go-atlas/reliability/outbox` |
| `ErrUndeliverable`| Marks a permanently undeliverable event so the outbox dead-letters it without retrying  |

## Deduplication

Outbox delivery is at-least-once, so the broker needs a stable identity to collapse repeats. On publish this adapter sets the `deduplicate_id` metadata
key to the outbox `Event.Id` — assigned once at save time and unchanged across every retry — which the NATS provider forwards as `Nats-Msg-Id`. A
nonempty caller-supplied `deduplicate_id` takes precedence. It is usually derived from the business entity, which dedupes across producers and not only across one
event's own retries.

The envelope retains the broker message TTL for each publish attempt. The outbox event's own expiry governs how long dispatch may be retried.

## Tracing

Supply `outbox.WithTracer(tracer)` to `New(store, publisher, opts...)` or `NewHandler(publisher, opts...)` to trace dispatch. Both accept
`outbox.WithMessagingSystem("nats")` for the optional system attribute. The broker factory's `UseTracer(tracer)` configures both its subscriber
and its outbox handlers, using the configured provider as the messaging system.

The envelope preserves a publishing request's W3C headers. For a sampled parent, the handler starts a PRODUCER span named `outbox send` and injects
the send context into the outgoing metadata. Its attributes include the destination topic, event ID, one-based attempt number and milliseconds since
event creation. A failed publish records an exception and error status. Unsampled parents pass through unchanged. Events without a valid parent
produce no dispatcher span.

The message creation timestamp stays in `message_created_time` metadata. The outbox records a new creation time at save, which anchors the event TTL.

## Ordering

Within a claimed batch, compaction keeps the latest saved event per topic. For relayed messages, the producer timestamp in
`message_created_time` remains available in metadata and does not affect compaction order.

To order several subjects for one aggregate, put the same `msg.MetaKeyOrderKey` in their message metadata. The adapter moves it into
`Event.OrderKey` and removes it from the envelope. Enable `sqldb.WithKeyOrder()` on dispatchers using the SQL store.

```go
m := msg.NewMessageWithMeta("events.updated."+id, payload, msg.Meta{
    {Key: msg.MetaKeyOrderKey, Value: "entity."+id},
})
err := b.Publish(sqldb.WithTx(ctx, tx), *m)
```

## Error classification

The adapter marks a stored payload it cannot decode with `ErrUndeliverable`. The event moves directly to `rejected`. A provider error matching
`broker.ErrConnectionClosed` stops new sends in the current cycle and reschedules the failed event without spending an attempt. Sends already in
progress can still finish. Other publish errors are transient and follow the ordinary retry budget.

## Methods

| Method        | Description                                                                                                       |
|---------------|-------------------------------------------------------------------------------------------------------------------|
| `New`         | Create a new broker outbox adapter with a `outbox.Storage`, `Publisher`, and broker outbox options  |
| `NewHandler`  | The `outbox.Handler` that decodes a stored broker envelope and publishes it as a `msg.Message`                    |
| `ShouldRetry` | The broker retry predicate for `outbox.WithShouldRetry` (see [Error classification](#error-classification))       |
| `Publish`     | Save a single message to the outbox store for later publishing                                                    |
| `PublishBatch`| Save multiple messages to the outbox store in a single operation                                                  |

The adapter also exposes these methods through its `Management` handle when constructed by `New` or the broker factory:

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

Pass storage, timeout, and scheduling options from [the generic outbox](../../../reliability/outbox) through `WithOutboxOptions`. Repeated calls append
options in order. The adapter applies its own retry predicate last.

```go
import dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"

ob := outbox.New(store, publisher, outbox.WithOutboxOptions(
    dataoutbox.WithLogger(logger),
    dataoutbox.WithHandleTimeout(5*time.Second),
    dataoutbox.WithMaxLockTime(time.Minute),
))
```

`WithMaxLockTime` must exceed `WithHandleTimeout`; a smaller value is raised at construction with a warning. Without `WithMeter` and execution of
the stats cycle (through `WithStatsSchedule`, a registered callback, or `RunStatsCycle`), the backlog, dead-letter, and lag gauges are not refreshed,
which is indistinguishable from a healthy idle outbox — see [docs/metrics.md](../../../docs/metrics.md#outbox).

## Scheduler registration

The management handle supplies `RegisterTasks(ctx)`. Its constructor does not install schedules; call this method after configuring
`WithScheduler` and handle errors. Partial registration gates callbacks until a retry on the same instance succeeds.
[Factory](../../../factory/messaging/broker) constructors register automatically and retain the outbox alongside any registration error.
