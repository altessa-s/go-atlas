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

New buckets are file-backed. Releases before this fix asked for file storage but created a memory bucket, which is lost when the JetStream
servers holding it stop. Replicated memory buckets survive rolling restarts, so they do not convert by themselves. The server cannot change a
bucket's storage type, so `New` adopts an existing bucket with a different storage type as is and logs a warning on every start.

To move to file storage, use the discard procedure:
1. Stop every process using the bucket.
2. Delete the bucket with `nats kv del <bucket>`.
3. Start the processes again. The first `New` creates a file bucket with the complete config, including the limit-marker TTL.
4. Check that `nats kv info <bucket>` reports file storage and that the warning is gone.

The bucket's keys are lost, so a retried request inside the old window can run again. Do not copy keys into the new bucket. A copy resets each
key's age, which gives completed keys a fresh retention window, and it drops the per-key TTL of in-progress locks.
