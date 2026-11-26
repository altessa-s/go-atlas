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
factory the switch is `storage.nats.migrateBucketTTL: true`. When migration changes `MaxAge`, the limit-marker TTL follows it. When `MaxAge` already
matches, migration can enable per-key TTL and limit markers in place while preserving the other bucket settings.

## Bucket storage

New buckets are file-backed. The server cannot change a bucket's storage type, so `New` returns `ErrBucketStorageMismatch` and leaves the bucket
untouched when it already exists with another one; align the configuration, recreate the bucket, or use another bucket.

## Orphan takeover

JetStream expires each in-progress lock lease. The Keeper uses the acquiring call's `MaxLockDuration` (default five minutes) or a shorter positive
`LockTTL`. The storage writes a per-key TTL rounded up to whole seconds. JetStream also applies the bucket's `MaxAge`. A direct call to
`Storage.AttemptLock` uses `MaxAge` alone. Pass a positive lease to `Storage.AttemptLockWithTTL` to set the per-key TTL explicitly.

After expiry, another owner can acquire a new revision and complete the same key. With the Keeper's default owner check, the original owner gets
`ErrOwnerMismatch` on retry until the replay record expires. Completion retains the replay record for the full bucket `MaxAge`.

The Keeper passes the holder's revision to `Storage.Complete` and `Storage.Release` to fence stale holders. A direct `Storage.Complete` call
with a nil token performs an unconditional write. `Steal` only reports the current value. Storage wrappers must forward `storages.LockLeaseStorage`
to apply the Keeper's lease cap.

`New` requires a file-backed bucket with per-key TTL enabled and one revision per key. On NATS 2.11 or later, use `WithMigrateBucketTTL()` to
resolve `ErrBucketPerKeyTTLRequired` for an existing bucket. `New` rejects mirrored buckets with
`ErrMirroredBucket` and multiple revisions per key with `ErrBucketHistoryMismatch`.

## Upgrade

Stop old writers before switching to lease-based writers. Existing records retain the TTL assigned when they were written. To make existing
orphans eligible sooner, remove their keys after confirming their holders are inactive. Completed entries can remain in the bucket.
