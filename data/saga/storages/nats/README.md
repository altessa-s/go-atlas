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
   completes, `New` fails with `ErrBucketMigrationInProgress` (a `KVMIGRATE_<bucket>` stream marks it), and a run without `Resume` refuses to
   continue it. A migrator holds a lease on the bucket (in the `kvmigrate_leases` bucket, renewed every 10s, expiring after 30s): a second
   migration, fresh or resumed, fails with `ErrBucketMigrationLocked` while one runs or until a crashed one's lease expires, and a migrator
   that lost its lease stops. The lease does not fence stream operations on the server, so still confirm a crashed run has exited before
   resuming. The bucket names `kvmigrate_leases` and `kvmigrate_*` are reserved.
4. Check that `nats kv info <bucket>` reports file storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

The migration seals the bucket, copies its live entries into the marker stream, recreates the bucket with its first revision just above the
old bucket's last one, and restores the entries. Revisions therefore keep growing across the migration. It rejects a JetStream context with a
domain or API prefix (`ErrMigrationUnsupportedContext`) and a bucket with sources, republishing, a subject transform or a placement
(`ErrMigrationUnsupportedBucket`). Recreating a sourced bucket would either replay old source values over newer local writes or lose
source messages not yet copied, since the server does not expose how far each source was consumed. Republishing would announce every restored
entry as a new change, and the others cannot be validated without a cluster.

A **mirror** bucket is migrated by re-sync. It is recreated as the same mirror on the new storage, nothing is copied, and the new mirror catches
up from its origin. It keeps the origin's revisions, so fencing stays monotonic. It holds what the origin still holds: entries the old mirror
kept but the origin has aged out are gone. Check the mirror's lag (`nats stream info KV_<bucket>`) before starting its readers.

A bucket key TTL other than the configured one fails with `ErrBucketTTLMismatch` unless `WithMigrateBucketTTL()` is passed too, which changes both at
once.

Every instance is kept, including terminal and `failed` ones. Its `Version` (`Execution.Fence`) is its new entry revision, so it stays above
any version handed out before, and fencing tokens kept by external systems remain valid. An instance lives the backstop TTL (`WithBucketTTL`)
from the migration on, which active instances renew at every checkpoint anyway.

Remaining lifetimes are computed from the server's message timestamps and the migrator's clock, so run the migration on a host whose clock is
synchronized with the NATS servers. A migrator clock ahead of the servers drops entries that still had that much lifetime left, and one behind
keeps entries that much longer. Transport delay between reading the clock and the server storing an entry adds the same kind of error, in the
order of a round trip.
