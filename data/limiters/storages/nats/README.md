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
