# outbox

```go
import "github.com/altessa-s/go-atlas/transport/broker/outbox"
```

Package `outbox` provides a broker-specific adapter for the generic transactional outbox pattern. Wraps `data/outbox` with
`msg.Message` conversion and NATS-specific retry logic, implementing the `broker.Outboxer` interface.

## Key types

| Type              | Description                                                                             |
|-------------------|------------------------------------------------------------------------------------------|
| `Outbox`          | Broker-specific adapter wrapping the generic `data/outbox.Outbox`                       |
| `Publisher`       | Interface for message transmission to external brokers                                  |
| `Store`           | Alias for `data/outbox.Store` interface                                                 |
| `Event`           | Alias for `data/outbox.Event` type                                                      |
| `Status`          | Alias for `data/outbox.Status` type                                                     |
| `Stats`           | Alias for `data/outbox.Stats` snapshot                                                  |
| `ErrUndeliverable`| Marks a permanently undeliverable event so the outbox dead-letters it without retrying  |

## Deduplication

Outbox delivery is at-least-once, so the broker needs a stable identity to collapse repeats. On publish this adapter sets the
`deduplicate_id` metadata key to the outbox `Event.Id` — assigned once at save time and unchanged across every retry — which the NATS
provider forwards as `Nats-Msg-Id`. A caller-supplied `deduplicate_id` always wins: it is usually derived from the business entity, which
dedupes across producers and not only across one event's own retries.

## Error classification

A stored payload this adapter cannot decode is wrapped in `ErrUndeliverable`, and the retry predicate reports it as permanent so the event
goes straight to `rejected` instead of consuming its whole retry budget. `nats.ErrConnectionClosed` and every other error stay transient and
are retried with backoff.

## Methods

| Method        | Description                                                                                  |
|---------------|----------------------------------------------------------------------------------------------|
| `New`         | Create a new broker outbox adapter with a `Store`, `Publisher`, and options                   |
| `Publish`     | Save a single message to the outbox store for later publishing                               |
| `PublishBatch`| Save multiple messages to the outbox store in a single operation                             |

## Options

| Option                        | Description                                                              |
|-------------------------------|--------------------------------------------------------------------------|
| `WithLogger`                  | Set the `*slog.Logger` for outbox operations                             |
| `WithContext`                 | Set the base context for background operations                           |
| `WithCollector`               | Metrics collector for the dispatch counters and backlog gauges           |
| `WithFetchTimeout`            | Maximum duration for fetching pending messages from the store            |
| `WithHandleTimeout`           | Maximum duration for publishing a single message to the broker           |
| `WithUpdateTimeout`           | Maximum duration for updating message status after a publish attempt     |
| `WithMaxLockTime`             | Lock lifetime before the unlock cycle reclaims a message                 |
| `WithMessagesBatchSize`       | Maximum number of messages fetched and dispatched per cycle              |
| `WithRetryMaxAttempts`        | Maximum publish attempts per message before dead-lettering               |
| `WithRetryBaseDelay`          | Backoff before the second publish attempt                                |
| `WithRetryMaxDelay`           | Backoff ceiling between publish attempts                                 |
| `WithMaxPayloadBytes`         | Message size limit; 0 disables the check                                 |
| `WithPublishedEventsLifetime` | Retention period for successfully published messages before cleanup      |
| `WithDefaultEventTTL`         | Default `ExpiresAt` applied at publish time                              |
| `WithTopicCompaction`         | Keep only the latest message per topic in a dispatch batch               |
| `WithTopicCompactionFilter`   | Enable topic compaction only for topics where the filter returns true    |
| `WithScheduler`               | Set the scheduler for the background cycles                              |
| `WithDispatchSchedule`        | Cron schedule for the dispatch cycle                                     |
| `WithUnlockSchedule`          | Cron schedule for the unlock cycle (releases stuck messages)             |
| `WithCleanupSchedule`         | Cron schedule for the cleanup cycle (deletes old published messages)     |
| `WithExpireSchedule`          | Cron schedule for the expire cycle                                       |
| `WithStatsSchedule`           | Cron schedule for the stats cycle (refreshes the backlog gauges)         |
| `With*TaskID`                 | Override a cycle's scheduler task ID — required when two outboxes share one scheduler |

`WithMaxLockTime` must exceed `WithHandleTimeout`; a smaller value is raised at construction with a warning. Without `WithStatsSchedule` and
`WithCollector` the backlog, dead-letter, and lag gauges stay at zero, which is indistinguishable from a healthy idle outbox — see
[docs/metrics.md](../../../docs/metrics.md#outbox).
