# outbox

```go
import "github.com/altessa-s/go-atlas/data/outbox"
```

Package `outbox` implements the Transactional Outbox pattern for at-least-once event delivery. Events are persisted to a `Store` before being
dispatched via a `Handler`, with background workers for dispatch, retry, cleanup, and stuck-event recovery. Transport-agnostic: the `Handler`
callback determines delivery method (message broker, HTTP, gRPC, etc.).

## Key types

| Type / Interface | Description                                              |
|------------------|----------------------------------------------------------|
| `Outbox`         | Main outbox manager with background workers              |
| `Event`          | Stored event with delivery tracking                      |
| `Status`         | Lifecycle: pending, in-progress, sent, failed, skipped   |
| `Handler`        | Callback that dispatches events                          |
| `Store`          | Persistence interface for event storage                  |

## Options

| Option                        | Default   | Description                            |
|-------------------------------|-----------|----------------------------------------|
| `WithFetchTimeout`            | 5s        | Store fetch timeout                    |
| `WithHandleTimeout`           | 20s       | Per-cycle dispatch timeout             |
| `WithUpdateTimeout`           | 5s        | Store update timeout                   |
| `WithEventsBatchSize`         | 200       | Events fetched per batch               |
| `WithRetryMaxAttempts`        | 10        | Maximum dispatch attempts              |
| `WithPublishedEventsLifetime` | indefinite| Retention for sent events              |
| `WithCompaction`              | off       | Deduplicate events by key in batch     |
| `WithCompactionFilter`        | nil       | Selective key-based compaction         |
| `WithShouldRetry`             | nil       | Custom retry predicate                 |
| `WithLogger`                  | discard   | Structured logger                      |

## Server-clock leases

The retry, lock-expiry, and retention windows passed to the `Store` are **durations**, not absolute timestamps: `FetchUnprocessedEvents(retryAfter)`,
`UnlockStuckEvents(lockExpiry)`, `DeleteProcessedEvents(olderThan)`, and `ExpireEvents`. The store evaluates them against its own database server clock,
so a worker whose wall clock is skewed cannot prematurely unlock another worker's in-flight event or leak a stuck one. See
[store/mongo](./store/mongo) for the MongoDB `$$NOW` implementation and the one-time BSON `Date` migration.

## Subpackages

| Package                          | Description                          |
|----------------------------------|--------------------------------------|
| [factory](./factory)             | Configuration-based creation         |
| [store/mongo](./store/mongo)     | MongoDB-backed event storage         |
