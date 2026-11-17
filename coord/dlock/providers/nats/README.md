# nats

```go
import "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
```

Package `nats` implements distributed lock provider using NATS JetStream key-value store with TTL-based lease management for automatic lock expiry on
holder failure.

## Constructors

| Constructor                            | Description                                                                   |
|----------------------------------------|-------------------------------------------------------------------------------|
| `New(ctx, conn, opts...)`              | The lock `Locker` provider, tuned by the options below                        |
| `NewDLock(ctx, conn, bucket, opts...)` | A `dlock.DLock` over a `Locker` on `bucket`; `opts` are `dlock.Option` values |

## Options

| Option                 | Default                   | Description                                                               |
|------------------------|---------------------------|---------------------------------------------------------------------------|
| `WithBucket`           | `<appinfo.Name>-dlock`    | KeyValue bucket name for the locks                                        |
| `WithTTL`              | 10s                       | Lock lease, and with it the key TTL of the bucket                         |
| `WithRenewRatio`       | 1/3                       | Fraction of the TTL at which the lease is renewed                         |
| `WithAcquireTimeout`   | 5s                        | Bound of the single acquisition attempt, separate from the lock's context |
| `WithStorage`          | `jetstream.MemoryStorage` | Storage type of the bucket                                                |
| `WithMigrateBucketTTL` | off                       | Update an existing bucket whose key TTL differs instead of failing        |
| `WithLogger`           | discard                   | Structured logger                                                         |

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so lock keys are escaped before they reach
the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). The lock key `orders:42` is
stored as `orders=3A42`; `LockInfo.Key` still reports `orders:42`. A key that is already valid and contains no `=` is stored under itself. The escaped
key may be at most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong` before any network I/O.

## Bucket TTL

The bucket's key TTL (`WithTTL`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already exists
with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes. Align the
configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through `factory/coord/dlock` the switch
is `distributionLock.nats.migrateBucketTTL: true`.

## Bucket storage

Lock buckets are memory-backed by default (`WithStorage`, `DefaultStorage`): a lock lives no longer than its TTL, so there is nothing to keep across a
server restart. Pass `WithStorage(jetstream.FileStorage)` (YAML: `distributionLock.nats.storage: file`) to keep the bucket, and with it the
fencing-token sequence, across a full server bounce. The server cannot change a bucket's storage type, so `New` returns `ErrBucketStorageMismatch`
and leaves the bucket untouched when it already exists with another one; align the configuration, recreate the bucket, or use another bucket.
