# nats

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
```

Package `nats` implements token bucket storage using NATS JetStream key-value store. Provides distributed rate limiting via NATS infrastructure.

## Bucket TTL

The bucket's key TTL (`WithMaxAge`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already
exists with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes.
Align the configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through the budget and
token-bucket limiter factories the switch is `storage.nats.migrateBucketTTL: true`.

## Bucket storage

New buckets are file-backed. Releases before this fix asked for file storage but created a memory bucket, which is lost when the JetStream servers
holding it stop. Replicated memory buckets survive rolling restarts, so they do not convert by themselves. The server cannot change a bucket's storage
type, so `New` adopts an existing bucket with a different storage type as is and logs a warning on every start. Pass `WithStrictBucketStorage()` (YAML:
`storage.nats.strictBucketStorage: true` through the limiter factories) to fail with `ErrBucketStorageMismatch` instead.

### Moving a bucket to file storage

1. Stop every process using the bucket.
2. Run a one-off program with the same options as production:

   ```go
   err := nats.MigrateBucketStorage(ctx, js, nats.MigrationOptions{}, opts...)
   ```

3. If it fails, fix the cause, confirm the run has exited, and rerun with `nats.MigrationOptions{Resume: true}`. Until the migration
   completes, `New` fails with `ErrBucketMigrationInProgress`. The migration lease that keeps a second migrator out, and the reserved bucket
   names, are described in the [shared procedure](../../../../docs/data/nats-kv-storage-migration.md#procedure).
4. Check that `nats kv info <bucket>` reports file storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

Counters keep their consumed budget. A counter lives the bucket TTL from the migration on; it carries its own request timestamps, so requests
older than the window are still ignored.

What the migration does, the buckets it rejects, how a mirror bucket is re-synced, and how accurately restored lifetimes follow the clocks are
described once for all NATS KV backends in [NATS KeyValue bucket storage migration](../../../../docs/data/nats-kv-storage-migration.md).
