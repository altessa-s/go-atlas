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
| `WithStrictBucketStorage` | off | Fail with `ErrBucketStorageMismatch` when the bucket exists with another storage type, instead of adopting it with a warning. YAML (saga factory): `storage.nats.strict_bucket_storage`. |

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

New buckets are file-backed. Releases before this fix asked for file storage but created a memory bucket, which is lost when the JetStream servers
holding it stop. Replicated memory buckets survive rolling restarts, so they do not convert by themselves. The server cannot change a bucket's storage
type, so `New` adopts an existing bucket with a different storage type as is and logs a warning on every start. Pass `WithStrictBucketStorage()` (YAML:
`storage.nats.strict_bucket_storage: true` through the saga factory) to fail with `ErrBucketStorageMismatch` instead.

### Moving a bucket to file storage

1. Stop every process using the bucket.
2. Run a one-off program with the same options as production:

   ```go
   err := natsstore.MigrateBucketStorage(ctx, js, natsstore.MigrationOptions{}, opts...)
   ```

3. If it fails, fix the cause, confirm the run has exited, and rerun with `natsstore.MigrationOptions{Resume: true}`. Until the migration
   completes, `New` fails with `ErrBucketMigrationInProgress`. The migration lease that keeps a second migrator out, and the reserved bucket
   names, are described in the [shared procedure](../../../../docs/data/nats-kv-storage-migration.md#procedure).
4. Check that `nats kv info <bucket>` reports file storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

Every instance is kept, including terminal and `failed` ones. Its `Version` (`Execution.Fence`) is its new entry revision, so it stays above
any version handed out before, and fencing tokens kept by external systems remain valid. An instance lives the backstop TTL (`WithBucketTTL`)
from the migration on, which active instances renew at every checkpoint anyway.

What the migration does, the buckets it rejects, how a mirror bucket is re-synced, and how accurately restored lifetimes follow the clocks are
described once for all NATS KV backends in [NATS KeyValue bucket storage migration](../../../../docs/data/nats-kv-storage-migration.md).
