# jetstream

```go
import sagajs "github.com/altessa-s/go-atlas/data/saga/engines/jetstream"
```

Drives saga executions from a NATS JetStream work queue. `Submit` durably enqueues a "start saga" command; `Run` consumes commands and drives each
through a [`saga.Orchestrator`](../../orchestrator.go) until the saga reaches a terminal state. Combined with the [NATS KV store](../../storages/nats)
it gives an all-NATS saga deployment in which any node can pick up any submitted saga.

## Key types

| Symbol                    | Description                                                                                                |
|---------------------------|------------------------------------------------------------------------------------------------------------|
| `Engine[T]`               | Binds a stream consumer to one `saga.Orchestrator[T]`. Safe for concurrent use.                            |
| `New(ctx, js, orch, ...)` | Creates the stream and durable consumer when absent; validates them otherwise.                             |
| `Engine.Submit`           | Publishes a start command for a saga ID and its initial data; returns after the JetStream publish ack.     |
| `Engine.Run`              | Consumes commands until the context ends; returns nil on shutdown, `ErrConsumeStopped` on unexpected stop. |

## Options

| Option                | Default         | Description                                                                                    |
|-----------------------|-----------------|------------------------------------------------------------------------------------------------|
| `WithStream`          | `SAGA_COMMANDS` | Stream name. One stream can carry several definitions.                                         |
| `WithSubjectPrefix`   | `saga.start`    | Command subject is `<prefix>.<definition name>`.                                               |
| `WithConsumer`        | `saga-<name>`   | Durable consumer shared by every engine of the definition.                                     |
| `WithConcurrency`     | 16              | Maximum sagas driven at once by one engine.                                                    |
| `WithAckWait`         | 30s             | AckWait for a newly created consumer. Heartbeats use a third of the consumer's actual AckWait. |
| `WithMaxDeliver`      | -1 (unlimited)  | Delivery cap for a newly created consumer.                                                     |
| `WithDuplicateWindow` | 2m              | De-duplication window for a newly created stream.                                              |
| `WithMaxAge`          | 0 (unlimited)   | Message age limit for a newly created stream.                                                  |
| `WithRetryBaseDelay`  | 1s              | First redelivery delay after an interrupted execution.                                         |
| `WithRetryMaxDelay`   | 1m              | Redelivery delay cap.                                                                          |
| `WithSerializer`      | JSON            | Codec for the saga data inside a command.                                                      |
| `WithLogger`          | discard         | Structured logger.                                                                             |
| `WithCollector`       | no-op           | Metrics collector (`saga_engine_*` counters and the `in_flight` gauge).                        |

Stream and consumer options apply only when `New` creates the resource. An existing stream or consumer is never modified, so one misconfigured
process cannot change retention or ack timing for the others.

## Behavior

| Outcome of `Orchestrator.Start`                                                              | Acknowledgement                                                                     |
|----------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------|
| Terminal instance (completed, compensated, failed, already terminal)                         | `Ack`                                                                               |
| Interrupted (store error, instance busy, non-terminal instance)                              | `NakWithDelay` (exponential)                                                        |
| Engine shutting down                                                                         | `NakWithDelay` (one fetch window, so the stopped engine's pull request has expired) |
| Panic while decoding or executing                                                            | `NakWithDelay` (exponential)                                                        |
| Undecodable command, empty or invalid-UTF-8 ID, definition mismatch, ID the KV store rejects | `Term`                                                                              |

- **De-duplication.** `Nats-Msg-Id` is `<len(definition)>:<definition>:<id>`: JetStream de-duplicates per stream, so the definition is part of the
  key and commands of different definitions never collapse into one. This does not namespace the **store**: saga stores key instances by ID
  alone, so definitions sharing a store must use distinct saga IDs. A command whose ID already belongs to another definition is terminated
  (`ErrDefinitionNotFound`).
- **Envelope.** A command is `{"id":"<saga id>","data":"<base64 of T encoded with WithSerializer>"}` on `<prefix>.<definition>`; other producers
  may publish it directly (the body must be valid UTF-8 JSON and `data` must be present).
- **Saga IDs.** Any non-empty valid UTF-8 string; `Submit` rejects anything else with `ErrInvalidID`, because the durable stores persist
  instances as JSON/BSON, which would rewrite such an ID. With the [NATS KV store](../../storages/nats) IDs must also be valid KV keys (letters,
  digits, `-_/=.`): a command whose ID the store rejects (`jetstream.ErrInvalidKey`) is terminated rather than retried forever.
- **De-duplication key encoding.** The key is base64url-encoded: header values are normalized on the wire (trailing whitespace trimmed, CR/LF
  replaced), which would otherwise map IDs such as `order` and `order ` to one key.
- **Fetching.** `Run` reserves free execution slots first and fetches at most that many commands, so a fetched command starts at once; nothing
  waits unheartbeated in a client buffer.
- **Existing consumers.** The engine adopts the consumer's actual settings: fetches respect `MaxRequestBatch` and `MaxRequestExpires`, and
  heartbeats follow the shortest delivery deadline (`AckWait`, or the smallest `BackOff` entry when the consumer has one).
- **Long sagas.** Running executions heartbeat with `InProgress` at a third of that deadline (at least 1ms).
- **Shutdown.** Canceling `Run`'s context cancels in-flight executions, which leave their instances non-terminal; their commands are returned to the
  queue. A closed connection makes `Run` cancel and join in-flight work at once — whether it was waiting for a slot, fetching, or backing off — and
  return `ErrConsumeStopped`. A deleted consumer is noticed by a pending fetch, or by the existence check that follows an empty fetch (a consumer
  deleted with no request outstanding sends no notice). `Run` then cancels and joins in-flight executions as on any unexpected stop — their
  instances stay non-terminal for a redelivery or the recovery cycle to resume — and returns `ErrConsumeStopped` joined with the cause. While
  every slot is busy no fetch is pending, so the deletion is noticed only once a slot frees.

## Errors

| Error                     | Returned when                                                                                                                                  |
|---------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `ErrInvalidSubject`       | The definition name or subject prefix cannot form a literal NATS subject.                                                                      |
| `ErrIncompatibleStream`   | The existing stream is not a work queue, does not cover the command subject, transforms subjects, skips publish acks (`NoAck`) or is a mirror. |
| `ErrIncompatibleConsumer` | The existing consumer filters another subject, lacks explicit acks, or is headers-only.                                                        |
| `ErrInvalidID`            | `Submit` got a saga ID that is not valid UTF-8.                                                                                                |
| `ErrAlreadyRunning`       | `Run` is called while another `Run` of the same engine is active.                                                                              |
| `ErrConsumeStopped`       | Consumption ended while `Run`'s context was still live.                                                                                        |

## Usage

```go
nc, _ := nats.Connect(nats.DefaultURL)
js, _ := jetstream.New(nc)
store, _ := natsstore.New(js)

orch := saga.New(store, def, saga.WithScheduler(sched))
if err := orch.RegisterRecovery(ctx); err != nil { // deadline rollback + crash recovery
	return err
}
engine, err := sagajs.New(ctx, js, orch, sagajs.WithConcurrency(8))
if err != nil {
	return err
}
go func() {
	if err := engine.Run(ctx); err != nil {
		logger.Error("saga engine stopped", "error", err)
	}
}()

err = engine.Submit(ctx, orderID, Order{ID: orderID})
```
