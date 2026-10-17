# msg

```go
import "github.com/altessa-s/go-atlas/transport/broker/msg"
```

Package `msg` defines the message envelope for broker systems. `Message` carries a topic, payload, `Meta` metadata, TTL, ack timeout, and an `Acker` for
acknowledgment. Data is copied on construction to ensure ownership.

## Key types

| Type       | Description                                                                                    |
|------------|------------------------------------------------------------------------------------------------|
| `Message`  | Single message with topic, `Data` payload, `Metadata`, TTL, `AckTimeout`, and `Acker`         |
| `Meta`     | Slice of `MetaData` entries with `Map`, `All`, and `Value` accessors                           |
| `MetaData` | Key-value pair for message metadata                                                            |
| `Acker`    | Interface for message acknowledgment: `Ack`, `Nak`, `NakWithBackOff`, `Term`, `InProgress`    |
| `BackOffFunc` | Redelivery delay as a function of the 1-based delivery attempt, used by `NakWithBackOff`    |

## Constructors

| Function           | Description                                                                      |
|--------------------|----------------------------------------------------------------------------------|
| `NewMessage`       | Create a message with topic and data; default metadata added automatically       |
| `NewMessageWithMeta`| Create a message with topic, data, and initial metadata entries                  |

## Message methods

| Method       | Description                                                                                 |
|--------------|---------------------------------------------------------------------------------------------|
| `Ack`        | Positive acknowledgment indicating successful processing                                    |
| `Nak`        | Negative acknowledgment with optional redelivery delay                                      |
| `NakWithBackOff` | Negative acknowledgment with a delay computed by a `BackOffFunc` from the delivery attempt |
| `Term`       | Terminal acknowledgment indicating the message should not be redelivered                     |
| `InProgress` | Heartbeat indicating processing is ongoing, preventing premature redelivery                  |

Each method returns `nil` without action when no `Acker` is set.

## Message options

| Option           | Description                                                                      |
|------------------|----------------------------------------------------------------------------------|
| `WithAcker`      | Set the `Acker` implementation for the message                                   |
| `WithAckTimeout` | Set the acknowledgment timeout (use `NoAckTimeout` for none)                     |
| `WithTTL`        | Set the Time-To-Live for the message                                             |

## Backoff helpers

| Function             | Description                                                                              |
|----------------------|------------------------------------------------------------------------------------------|
| `ScheduleBackOff`    | `BackOffFunc` picking delays from a fixed schedule; the last delay repeats               |
| `ExponentialBackOff` | `BackOffFunc` with exponential growth (factor 1.5) capped at `maxDelay`, plus jitter     |

## Metadata helpers

| Function                   | Description                                                              |
|----------------------------|--------------------------------------------------------------------------|
| `MetaFromMap`              | Convert a `map[string]string` into a `Meta` slice                        |
| `MessageCreatedTimeFromMeta`| Extract and parse the creation timestamp from metadata                  |
| `MessageIdFromMeta`        | Extract the message ID string from metadata                              |

## Constants

| Constant                    | Description                                                              |
|-----------------------------|--------------------------------------------------------------------------|
| `NoAckTimeout`              | Sentinel duration indicating no acknowledgment timeout                   |
| `MetaKeyDeduplicateId`      | Metadata key for deduplication ID (see [Deduplication](#deduplication))  |
| `MetaKeyMessageId`          | Metadata key for unique message identifier                               |
| `MetaKeyMessageCreatedTime` | Metadata key for message creation timestamp                              |
| `MessageCreatedTimeFormat`  | Time format (RFC 3339) used for the creation timestamp                   |

## Deduplication

Setting `MetaKeyDeduplicateId` on a message makes the NATS provider publish it with that value as `Nats-Msg-Id`, so JetStream drops a duplicate publish
of the same identifier. The subscriber maps `Nats-Msg-Id` back to `MetaKeyDeduplicateId` metadata, so consumers — the `transport/broker/idempotency`
middleware's default key among them — see the identifier on every delivery and redelivery; an explicit `deduplicate_id` application header from another
publisher takes precedence.

**It reduces duplicates; it does not eliminate them.** The server only remembers an identifier for the length of the stream's duplicate window
(`StreamConfig.Duplicates`, which the caller sets when creating the stream). A retry that arrives after that window is a fresh message to the server and
is stored again — and a window wide enough to cover every conceivable retry costs memory on every stream.

So treat it as an optimization, not a guarantee: the consumer still has to be safe to run twice. Make the operation naturally idempotent, or protect it
with a unique key on the business identifier, or record processed identifiers (`data/idempotency`). Deduplication lowers how often a duplicate shows up;
only idempotency makes one harmless.
