# nats

```go
import "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
```

Package `nats` implements distributed lock provider using NATS JetStream key-value store with TTL-based lease management for automatic lock expiry
on holder failure.

## Bucket TTL

The bucket's key TTL (`WithTTL`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already exists
with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes. Align the
configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through `dlock/factory` the switch is
`distributionLock.nats.migrateBucketTTL: true`.

## Bucket storage

Lock buckets are memory-backed: a lock lives no longer than its TTL, so there is nothing to keep across a server restart. An existing bucket with
another storage type is adopted as is, with a warning, because the server cannot change a bucket's storage type.
