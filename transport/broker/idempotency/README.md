# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/broker/idempotency"
```

Package `idempotency` provides deduplication middleware for broker subscribers backed by [`data/idempotency.Keeper`](../../../data/idempotency). It
offers effectively-once processing semantics over at-least-once message delivery without altering subscriber handler contracts.

## Key types

| Type / Function | Description |
|-----------------|-------------|
| `Wrap` | Wraps a `broker.SubscriberHandler` with deduplication scoped by consumer name |
| `KeyFunc` | Function signature for extracting an idempotency key from an incoming message |
| `DefaultKeyFunc` | Extracts the key from message metadata (`deduplicate_id`, then `message_id`) |
| `ProtoKeyFunc` | Generic helper extracting the key from a deserialized protobuf message payload |
| `BuildStorageKey` | Composes the `("ibk", consumer, key)` storage key in the v2 format (`idk:v2:3:ibk7:billing9:invoice-1`) |
| `StorageKeyPrefix` | `"ibk"`, the domain tag that keeps broker keys apart from the HTTP and gRPC transport keys |

## Storage keys

`BuildStorageKey` composes its tuple with [`data/idempotency.ComposeKey`](../../../data/idempotency#storage-key-format): every component is
length-prefixed, so distinct `(consumer, key)` pairs never share a record whatever bytes they hold. The v2 format replaced the colliding
`ibk:{consumer}:{key}` layout with a clean cut: v1 records are never read, so follow the
[rollout contract](../../../data/idempotency#rolling-out-the-v2-key-format) when upgrading.

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithKeyFunc` | `DefaultKeyFunc` | Custom key extraction strategy |
| `WithBusyBackOff` | `DefaultBusyBackOff` | Backoff schedule for `NakWithBackOff` when a key is already in progress |
| `WithErrorBackOff` | `DefaultErrorBackOff` | Backoff schedule for `NakWithBackOff` upon transient storage errors |
| `WithLockOpts` | zero | Per-call `idempotency.AttemptLockOpts` (e.g. `LockTTL`, `MaxLockDuration`) |
| `WithLogger` | discard | Logger for warnings, key errors, and storage failures |
| `WithCollector` / `WithMetrics` | no-op | Metrics collector for `broker_idempotency_outcomes_total` (labels `consumer`, `outcome`) |

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
