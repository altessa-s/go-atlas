# nats

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
```

Package `nats` implements idempotency storage using NATS JetStream key-value store. Provides distributed idempotency detection with TTL support
via NATS infrastructure.

## Bucket TTL

The bucket's key TTL (`WithMaxAge`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already
exists with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes.
Align the configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through the idempotency
factory the switch is `storage.nats.migrateBucketTTL: true`. Migration applies the whole bucket config, so the limit-marker TTL moves to the new
`WithMaxAge` as well, and a bucket created without one gets per-key TTL enabled (NATS server 2.11+).

## Bucket storage

New buckets are file-backed. Releases before this fix asked for file storage but created a memory bucket, which is lost when the JetStream servers
holding it stop. Replicated memory buckets survive rolling restarts, so they do not convert by themselves. The server cannot change a bucket's storage
type, so `New` adopts an existing bucket with a different storage type as is and logs a warning on every start. Pass `WithStrictBucketStorage()` (YAML:
`storage.nats.strictBucketStorage: true` through the idempotency factory) to fail with `ErrBucketStorageMismatch` instead.

### Moving a bucket to file storage

1. Stop every process using the bucket.
2. Run a one-off program with the same options as production:

   ```go
   err := nats.MigrateBucketStorage(ctx, js, nats.MigrationOptions{}, opts...)
   ```

3. If it fails, fix the cause, confirm the run has exited, and rerun with `nats.MigrationOptions{Resume: true}`. Until the migration
   completes, `New` fails with `ErrBucketMigrationInProgress` (a `KVMIGRATE_<bucket>` stream marks it), and a run without `Resume` refuses to
   continue it. Never run two migrations of one bucket at once.
4. Check that `nats kv info <bucket>` reports file storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

The migration seals the bucket, copies its live entries into the marker stream, recreates the bucket with its first revision just above the
old bucket's last one, and restores the entries. Revisions therefore keep growing across the migration. It rejects a JetStream context with a
domain or API prefix (`ErrMigrationUnsupportedContext`) and a bucket with a mirror, sources, republishing, a subject transform or a placement
(`ErrMigrationUnsupportedBucket`). A bucket key TTL other than the configured one fails with `ErrBucketTTLMismatch` unless `WithMigrateBucketTTL()`
is passed too, which changes both at once.

Expiry is kept as far as the bucket allows. An in-progress lock keeps its per-key deadline (rounded up to the next second, within the clock accuracy
below). A completed key lives the bucket TTL from the migration on, so it is remembered at most its elapsed age longer, which only widens deduplication.
Entries already past their deadline are not restored.

Remaining lifetimes are computed from the server's message timestamps and the migrator's clock, so run the migration on a host whose clock is
synchronized with the NATS servers. A migrator clock ahead of the servers drops entries that still had that much lifetime left, and one behind
keeps entries that much longer. Transport delay between reading the clock and the server storing an entry adds the same kind of error, in the
order of a round trip.
