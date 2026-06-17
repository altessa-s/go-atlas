# nats

```go
import natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
```

Durable [`saga.Store`](../../store.go) backed by a NATS JetStream KeyValue bucket. Each instance is a JSON document keyed by its ID, and the bucket's
revision is used directly as the optimistic-concurrency token (`saga.Instance.Version`) — so two coordinators cannot advance the same instance, and state
survives process restarts for crash recovery.

## Options

| Option            | Default | Description                                                                                  |
|-------------------|---------|----------------------------------------------------------------------------------------------|
| `WithBucket`      | `saga`  | KeyValue bucket name.                                                                         |
| `WithBucketTTL`   | 30 days | Backstop time-to-live for instances. Active sagas reset it on every checkpoint; completed ones are deleted explicitly. Tune to your longest saga lifetime plus retention. |

## Behavior

| Method             | Notes                                                                                          |
|--------------------|------------------------------------------------------------------------------------------------|
| `Create`           | `KV.Create`; `errs.ErrInstanceExists` on a duplicate ID. Writes the new revision into `Version`. |
| `Get`              | `errs.ErrInstanceNotFound` when absent; `Version` reflects the current revision.               |
| `Update`           | Revision-checked `KV.Update`; `errs.ErrVersionConflict` on a stale `Version`, `errs.ErrInstanceNotFound` when gone. |
| `FetchRecoverable` | Scans every key (NATS KV has no queries) and returns non-terminal instances mid-compensation or past their deadline. |
| `Delete`           | Idempotent.                                                                                     |

Instance IDs are used verbatim as KV keys, so they must be valid NATS KV keys (letters, digits, `-_/=.`); ULIDs and UUIDs qualify.

## Usage

```go
nc, _ := nats.Connect(nats.DefaultURL)
js, _ := jetstream.New(nc)
store, err := natsstore.New(js, natsstore.WithBucket("saga"))

orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
inst, err := orch.Start(ctx, id, data)
```
