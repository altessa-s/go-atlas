# idempotency

```go
import "github.com/altessa-s/go-atlas/messaging/broker/idempotency"
```

Package `idempotency` provides deduplication middleware for broker subscribers backed by the local [`Keeper` interface](#key-types). The
[`reliability/idempotency.Keeper`](../../../reliability/idempotency) implements this contract. The middleware offers effectively-once processing semantics
over at-least-once message delivery without altering subscriber handler contracts.

## Key types

| Type / Function | Description |
|-----------------|-------------|
| `Wrap` | Wraps a `broker.SubscriberHandler` with deduplication scoped by consumer name |
| `Keeper` | Consumer-side port for acquiring locks, completing records, and releasing locks |
| `KeyFunc` | Function signature for extracting an idempotency key from an incoming message |
| `DefaultKeyFunc` | Extracts the key from message metadata (`deduplicate_id`, then `message_id`) |
| `ProtoKeyFunc` | Generic helper extracting the key from a deserialized protobuf message payload |
| `BuildStorageKey` | Composes the `("ibk", consumer, topic, key)` storage key in the v2 format (`idk:v2:3:ibk7:billing8:invoices9:invoice-1`) |
| `ErrKeyTooLong` | Terminates a message whose key exceeds `WithMaxKeyLength` |
| `StorageKeyPrefix` | `"ibk"`, the domain tag that keeps broker keys apart from the HTTP and gRPC transport keys |

## Storage keys

`BuildStorageKey` composes its tuple with [`reliability/idempotency.ComposeKey`](../../../reliability/idempotency#storage-key-format): every component is
length-prefixed, so distinct `(consumer, topic, key)` triples never share a record whatever bytes they hold. `topic` is the subject the message
arrived on (`msg.Message.Topic`), not the subscription pattern, so records are scoped by subject. Keys over `WithMaxKeyLength` bytes (default
`DefaultMaxKeyLength`, 128) are terminated with `ErrKeyTooLong` instead of being stored.

## Trust model

The dedup key is chosen by the producer (`deduplicate_id` / `message_id` metadata, or a payload field), and the broker carries no authenticated
producer identity the middleware could bind it to. Any publisher allowed on a subject can therefore send a message with the dedup ID of a later
message on that subject, and the later message is acknowledged as a duplicate without running its handler. The middleware limits the reach of
such a message:

- records are scoped by consumer and by the concrete subject, so a publisher on one subject (`orders.tenant-b`) never suppresses a message of
  another (`orders.tenant-a`), even when one consumer subscribes to both through a wildcard;
- keys are capped (`WithMaxKeyLength`), so a publisher cannot turn metadata of arbitrary size into storage keys.

Within one subject, every permitted publisher is trusted. Restrict publish permissions per subject (NATS accounts and user permissions), give
each tenant or producer its own subject, or use a `KeyFunc` that derives the key from a value the producer cannot forge (a signed payload field
verified first) when publishers do not trust each other.

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithKeyFunc` | `DefaultKeyFunc` | Custom key extraction strategy |
| `WithBusyBackOff` | `DefaultBusyBackOff` | Backoff schedule for `NakWithBackOff` when a key is already in progress |
| `WithErrorBackOff` | `DefaultErrorBackOff` | Backoff schedule for `NakWithBackOff` upon transient storage errors |
| `WithLockOpts` | zero | Per-call `idempotency.AttemptLockOpts` (e.g. `LockTTL`, `MaxLockDuration`) |
| `WithMaxKeyLength` | `DefaultMaxKeyLength` (128) | Longest key in bytes; a longer one is terminated with `ErrKeyTooLong`; non-positive is the default |
| `WithLogger` | discard | Logger for warnings, key errors, and storage failures |
| `WithMeter` | no-op | Metrics meter for `broker_idempotency_outcomes_total` (labels `consumer`, `outcome`) |

## Usage

### Metadata-based deduplication

```go
handler := brokeridem.Wrap(myHandler, keeper, "sms_consumer",
    brokeridem.WithKeyFunc(brokeridem.DefaultKeyFunc),
)
```

### Payload-based protobuf deduplication

When the business identifier resides within the protobuf payload rather than broker headers:

```go
handler := brokeridem.Wrap(myHandler, keeper, "email_consumer",
    brokeridem.WithKeyFunc(brokeridem.ProtoKeyFunc(
        func() *emailv1.SendEmailRequest { return new(emailv1.SendEmailRequest) },
        (*emailv1.SendEmailRequest).GetMessageId,
    )),
)
```

## Delivery semantics

| Condition | Action | Handler called |
|-----------|--------|----------------|
| `KeyFunc` returns empty key | Pass-through with warning and `no_key` metric | Yes |
| `KeyFunc` returns error | Terminal rejection (`Term`) with `key_error` metric | No |
| Key longer than `WithMaxKeyLength` | Terminal rejection (`Term`, `ErrKeyTooLong`) with `key_error` metric | No |
| `AttemptLock` storage error | Fail closed (`NakWithBackOff`) with `storage_error` metric | No |
| Lock acquired | Handler invoked with decorated acker | Yes |
| Key already `SUCCESS` | Immediate acknowledgment (`Ack`) with `duplicate` metric | No |
| Key is `IN_PROGRESS` | Redelivery delay (`NakWithBackOff`) with `busy` metric | No |

Upon completion:
- **`Ack`**: marks the lock as complete (`Complete`, `processed` metric) and forwards `Ack`; a `Complete` failure is logged and counted as
  `lock_stolen` (`ErrLockStolen`) or `storage_error`, and `Ack` is still forwarded.
- **`Nak` / `NakWithBackOff`**: releases the lock (`Release`) and forwards negative acknowledgment for redelivery.
- **`Term`**: releases the lock (`Release`) and forwards terminal acknowledgment.
- **Panic**: releases the lock (`Release`) and re-panics so the broker runtime can record failure.
