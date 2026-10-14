# nats

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
```

Package `nats` implements distributed lock provider using NATS JetStream key-value store with TTL-based lease management for automatic lock expiry
on holder failure.

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so lock keys are escaped before they reach
the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). The lock key `orders:42` is
stored as `orders=3A42`; `LockInfo.Key` still reports `orders:42`. A key that is already valid and contains no `=` is stored under itself, so existing
data stays reachable. A key containing `=` now maps to another physical key than the raw one earlier releases wrote; migrate such entries or start on a
fresh bucket. Upgrade every process sharing the bucket together: an old and a new process would otherwise address such a key under different physical
keys. The escaped key may be at most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong` before any network I/O.

## Bucket TTL

The bucket's key TTL (`WithTTL`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already exists
with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes. Align the
configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through `dlock/factory` the switch is
`distributionLock.nats.migrateBucketTTL: true`.

## Bucket storage

Lock buckets are memory-backed by default (`WithStorage`, `DefaultStorage`): a lock lives no longer than its TTL, so there is nothing to keep across a
server restart. Pass `WithStorage(jetstream.FileStorage)` (YAML: `distributionLock.nats.storage: file`) to keep the bucket, and with it the
fencing-token sequence, across a full server bounce. An existing bucket with another storage type is adopted as is, with a warning, because the server
cannot change a bucket's storage type. Pass `WithStrictBucketStorage()` (YAML: `distributionLock.nats.strictBucketStorage: true`) to fail with
`ErrBucketStorageMismatch` instead.

### Moving a bucket to another storage type

1. Stop every process using the bucket.
2. Run a one-off program with the same options as production:

   ```go
   err := nats.MigrateBucketStorage(ctx, nc, nats.MigrationOptions{}, opts...)
   ```

3. If it fails, fix the cause, confirm the run has exited, and rerun with `nats.MigrationOptions{Resume: true}`. Until the migration
   completes, `New` fails with `ErrBucketMigrationInProgress`. The migration lease that keeps a second migrator out, and the reserved bucket
   names, are described in the [shared procedure](../../../../../docs/data/nats-kv-storage-migration.md#procedure).
4. Check that `nats kv info <bucket>` reports the new storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

The migration recreates the bucket with its first revision just above the old bucket's last one, so fencing tokens (`LockInfo.FencingToken`)
keep growing. Locks are not copied: with every user stopped none is held, and copying one would revive it for a full TTL. The buckets the
migration rejects (`ErrMigrationUnsupportedContext`, `ErrMigrationUnsupportedBucket`, `ErrBucketTTLMismatch`) and how a mirror bucket is re-synced
are described once for all NATS KV backends in [NATS KeyValue bucket storage migration](../../../../../docs/data/nats-kv-storage-migration.md).
