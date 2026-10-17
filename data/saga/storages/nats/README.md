# nats

```go
import natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
```

Durable [`saga.Storage`](../../storage.go) backed by a NATS JetStream KeyValue bucket. Each instance is a JSON document keyed by its ID, and the
bucket's revision is used directly as the optimistic-concurrency token (`saga.Instance.Version`) — so two coordinators cannot advance the same instance,
and state survives process restarts for crash recovery.

## Options

| Option            | Default | Description                                                                                  |
|-------------------|---------|----------------------------------------------------------------------------------------------|
| `WithBucket`      | `saga`  | KeyValue bucket name.                                                                         |
| `WithBucketTTL`   | 30 days | Backstop time-to-live for instances. Active sagas reset it on every checkpoint; terminal ones stay until the TTL expires or the caller invokes `Delete`. Tune to your longest saga lifetime plus retention. |
| `WithMigrateBucketTTL` | off | Update an existing bucket whose key TTL differs from `WithBucketTTL`. Without it `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched. YAML (saga factory): `storage.nats.migrateBucketTTL`. |
| `WithStrictBucketStorage` | off | Fail with `ErrBucketStorageMismatch` when the bucket exists with another storage type, instead of adopting it with a warning. YAML (saga factory): `storage.nats.strictBucketStorage`. |

## Behavior

| Method             | Notes                                                                                          |
|--------------------|------------------------------------------------------------------------------------------------|
| `Create`           | `KV.Create`; `errs.ErrInstanceExists` on a duplicate ID. Writes the new revision into `Version`. |
| `Get`              | `errs.ErrInstanceNotFound` when absent; `Version` reflects the current revision.               |
| `Update`           | Revision-checked `KV.Update`; `errs.ErrVersionConflict` on a stale `Version`, `errs.ErrInstanceNotFound` when gone. |
| `FetchRecoverable` | Scans every key (NATS KV has no queries) and returns non-terminal instances mid-compensation, past their deadline, or with an expired execution lease. |
| `Delete`           | Idempotent.                                                                                     |

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so instance IDs are escaped before they
reach the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). ULIDs and UUIDs are stored
as they are; `order:42` is stored as `order=3A42`. A key that is already valid and contains no `=` is stored under itself, so existing data stays
reachable. A key containing `=` now maps to another physical key than the raw one earlier releases wrote; migrate such entries or start on a fresh
bucket. The escaped key may be at most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong` before any network I/O. An ID
that is not valid UTF-8 fails with `ErrInvalidID`: the instance is stored as JSON, which would rewrite the ID.

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
`storage.nats.strictBucketStorage: true` through the saga factory) to fail with `ErrBucketStorageMismatch` instead.

### Moving a bucket to file storage

1. Stop every process using the bucket.
2. Run a one-off program with the same options as production:

   ```go
   err := natsstore.MigrateBucketStorage(ctx, js, natsstore.MigrationOptions{}, opts...)
   ```

3. If it fails, fix the cause, confirm the run has exited, and rerun with `natsstore.MigrationOptions{Resume: true}`. Until the migration completes,
   `New` fails with `ErrBucketMigrationInProgress`. The migration lease that keeps a second migrator out, and the reserved bucket names, are described
   in the [shared procedure](../../../../docs/data/nats-kv-storage-migration.md#procedure).
4. Check that `nats kv info <bucket>` reports file storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

Every instance is kept, including terminal and `failed` ones. Its `Version` (`Execution.Fence`) is its new entry revision, so it stays above any version
handed out before, and fencing tokens kept by external systems remain valid. An instance lives the backstop TTL (`WithBucketTTL`) from the migration on,
which active instances renew at every checkpoint anyway.

What the migration does, the buckets it rejects, how a mirror bucket is re-synced, and how accurately restored lifetimes follow the clocks are described
once for all NATS KV backends in [NATS KeyValue bucket storage migration](../../../../docs/data/nats-kv-storage-migration.md).
