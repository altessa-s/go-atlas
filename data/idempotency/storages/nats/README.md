# nats

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
```

Package `nats` implements idempotency storage using NATS JetStream key-value store. Provides distributed idempotency detection with TTL support via NATS
infrastructure.

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so logical keys are escaped before they
reach the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). The key the HTTP middleware
builds for `POST /orders` with key `123`, which the `Keeper` stores as `idk:v2:9:namespace0:38:idk:v2:9:transport12:POST:/orders3:123` (see the
[storage-key format](../../README.md#storage-key-format)), is stored as
`idk=3Av2=3A9=3Anamespace0=3A38=3Aidk=3Av2=3A9=3Atransport12=3APOST=3A/orders3=3A123`. The key is escaped once, after both layers have composed it. As
far as the escaping goes, a key that is already valid and contains no `=` is stored under itself, and a key containing `=` maps to another physical key
than the raw one earlier releases wrote, so an old and a new process sharing a bucket would address such a key under different physical keys. The v2 key
format itself is a clean cut that never reads earlier records: run it on a fresh bucket and follow the
[rollout contract](../../README.md#rolling-out-the-v2-key-format). The escaped key may be at most 3072 bytes (an escaped byte takes three); a longer one
fails with `ErrKeyTooLong` before any network I/O.

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

3. If it fails, fix the cause, confirm the run has exited, and rerun with `nats.MigrationOptions{Resume: true}`. Until the migration completes, `New`
   fails with `ErrBucketMigrationInProgress`. The migration lease that keeps a second migrator out, and the reserved bucket names, are described in the
   [shared procedure](../../../../docs/data/nats-kv-storage-migration.md#procedure).
4. Check that `nats kv info <bucket>` reports file storage and that the `KVMIGRATE_<bucket>` stream is gone.
5. Start the processes.

Expiry is kept as far as the bucket allows. An in-progress lock keeps its per-key deadline (rounded up to the next second, within the clock accuracy
described in the shared migration doc). A completed key lives the bucket TTL from the migration on, so it is remembered at most its elapsed age longer,
which only widens deduplication. Entries already past their deadline are not restored.

What the migration does, the buckets it rejects, how a mirror bucket is re-synced, and how accurately restored lifetimes follow the clocks are described
once for all NATS KV backends in [NATS KeyValue bucket storage migration](../../../../docs/data/nats-kv-storage-migration.md).
