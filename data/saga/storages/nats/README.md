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
| `WithMigrateBucketTTL` | off | Update an existing bucket whose key TTL differs from `WithBucketTTL`. Without it `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched. YAML (saga factory): `storage.nats.migrate_bucket_ttl`. |

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

## Bucket storage

New buckets are file-backed. Releases before this fix asked for file storage but created a memory bucket, which is lost when the JetStream
servers holding it stop. Replicated memory buckets survive rolling restarts, so they do not convert by themselves. The server cannot change a
bucket's storage type, so `New` adopts an existing bucket with a different storage type as is and logs a warning on every start.

Moving to file storage recreates the bucket, which resets its KV revisions, and an instance's `Version` (`Execution.Fence`) is its entry
revision. If an external system keeps the highest fence it has accepted, do not migrate: keep the adopted bucket until a fencing-safe migration
exists. Otherwise pick one of the two procedures below.

**Discard.** Use it only when no instance is `running` or `compensating`, no `failed` instance is awaiting manual intervention, and you accept
losing the terminal records. Once its record is gone, `Start` with the same ID runs a new instance, so a completed saga could be replayed.
1. Stop every process using the bucket.
2. Delete the bucket with `nats kv del <bucket>`.
3. Start the processes again. The first `New` creates a file bucket.
4. Check that `nats kv info <bucket>` reports file storage and that the warning is gone.

**Preserve.** Use it in every other case.
1. Stop every process using the bucket. Keep them stopped until the last step.
2. Copy every key to a temporary bucket.
3. Delete the bucket with `nats kv del <bucket>`.
4. Run a one-off program that calls `New` with the production options. It creates the file bucket.
5. Copy the keys back.
6. Verify the key count, the file storage and a sample of instances.
7. Delete the temporary bucket.
8. Resume the processes.

A copy resets each key's age, so every instance starts a fresh backstop TTL (`WithBucketTTL`). This is harmless because active instances reset
the TTL on every checkpoint anyway.
