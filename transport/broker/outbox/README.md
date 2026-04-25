# outbox

```go
import "github.com/altessa-s/go-atlas/transport/broker/outbox"
```

Package `outbox` provides a broker-specific adapter for the generic transactional outbox pattern. Wraps `data/outbox` with
`msg.Message` conversion and NATS-specific retry logic, implementing the `broker.Outboxer` interface.

## Key types

| Type        | Description                                                                                   |
|-------------|-----------------------------------------------------------------------------------------------|
| `Outbox`    | Broker-specific adapter wrapping the generic `data/outbox.Outbox`                             |
| `Publisher` | Interface for message transmission to external brokers                                        |
| `Store`     | Alias for `data/outbox.Store` interface                                                       |
| `Event`     | Alias for `data/outbox.Event` type                                                            |
| `Status`    | Alias for `data/outbox.Status` type                                                           |

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
| `WithFetchTimeout`           | Maximum duration for fetching pending messages from the store            |
| `WithHandleTimeout`          | Maximum duration for publishing a single message to the broker           |
| `WithUpdateTimeout`          | Maximum duration for updating message status after a publish attempt     |
| `WithMessagesBatchSize`      | Maximum number of messages fetched and dispatched per cycle              |
| `WithRetryMaxAttempts`       | Maximum publish attempts per message before marking as failed            |
| `WithPublishedEventsLifetime`| Retention period for successfully published messages before cleanup      |
| `WithTopicCompaction`        | Keep only the latest message per topic in a dispatch batch               |
| `WithTopicCompactionFilter`  | Enable topic compaction only for topics where the filter returns true    |
| `WithScheduler`              | Set the scheduler for background dispatch, unlock, and cleanup tasks     |
| `WithDispatchSchedule`       | Cron schedule for the dispatch cycle                                     |
| `WithUnlockSchedule`         | Cron schedule for the unlock cycle (releases stuck messages)             |
| `WithCleanupSchedule`        | Cron schedule for the cleanup cycle (deletes old published messages)     |
