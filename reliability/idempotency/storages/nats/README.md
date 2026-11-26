# nats

```go
import "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
```

Package `nats` implements idempotency storage using NATS JetStream key-value store. Provides distributed idempotency detection with TTL support via NATS
infrastructure.

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so logical keys are escaped before they
reach the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). The key the HTTP middleware
builds for `POST /orders` with key `123`, which the `Keeper` stores as `idk:v2:9:namespace0:38:idk:v2:9:transport12:POST:/orders3:123` (see the
[storage-key format](../../README.md#storage-key-format)), is stored as
`idk=3Av2=3A9=3Anamespace0=3A38=3Aidk=3Av2=3A9=3Atransport12=3APOST=3A/orders3=3A123`. The key is escaped once, after both layers have composed it. A
key that is already valid and contains no `=` is stored under itself. The escaped key may be at most 3072 bytes (an escaped byte takes three); a longer
one fails with `ErrKeyTooLong` before any network I/O.

## Bucket TTL

The bucket's key TTL (`WithMaxAge`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already
exists with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes.
Align the configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through the idempotency
factory the switch is `storage.nats.migrateBucketTTL: true`. Migration applies the whole bucket config, so the limit-marker TTL moves to the new
`WithMaxAge` as well, and a bucket created without one gets per-key TTL enabled (NATS server 2.11+).

## Bucket storage

New buckets are file-backed. The server cannot change a bucket's storage type, so `New` returns `ErrBucketStorageMismatch` and leaves the bucket
untouched when it already exists with another one; align the configuration, recreate the bucket, or use another bucket.

## Orphan takeover

Orphan takeover uses the Keeper clock and requires synchronized clocks across instances sharing the bucket. A positive per-call lock TTL applies to both initial acquisition and takeover.
