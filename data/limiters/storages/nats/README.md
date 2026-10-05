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

New buckets are file-backed. Releases before this fix asked for file storage but created a memory bucket, which is lost when the JetStream
servers holding it stop. Replicated memory buckets survive rolling restarts, so they do not convert by themselves. The server cannot change a
bucket's storage type, so `New` adopts an existing bucket with a different storage type as is and logs a warning on every start.

To move to file storage:
1. Stop every process using the bucket.
2. Delete the bucket with `nats kv del <bucket>`.
3. Start the processes again. The first `New` creates a file bucket.
4. Check that `nats kv info <bucket>` reports file storage and that the warning is gone.

The rate-limit counters start from zero after the move.
