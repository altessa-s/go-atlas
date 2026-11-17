# nats

```go
import "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
```

Package `nats` implements token bucket storage using NATS JetStream key-value store. Provides distributed rate limiting via NATS infrastructure.
`New(ctx, js, opts...)` creates the bucket on `ctx` when missing: `WithBucket` (default `<appinfo.Name>-rate-limiter`), `WithMaxAge` (key TTL, `24h`)
and `WithReplicas` (`1`).

## Multi-key admission

NATS KeyValue updates one key per compare-and-swap and cannot update several keys atomically, so `Provider` does not implement
`storages.MultiStorage`: `storages.AllowAll` returns `storages.ErrMultiKeyUnsupported` for it rather than taking slots key by key. Use the memory or
redis storage where one request must pass several limits at once.

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so limiter keys are escaped before they
reach the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). An IPv6 client key
`2001:db8::1` is stored as `2001=3Adb8=3A=3A1`. A key that is already valid and contains no `=` is stored under itself. The escaped key may be at
most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong` before any network I/O.

## Contention

`Allow` updates a key with a compare-and-swap on its revision. A write that loses the race to another caller is re-read, recomputed and retried after
a
randomized pause (full jitter, from 1 ms doubling up to `WithMaxCASBackoff`, 20 ms by default), so a hot key costs latency rather than errors;
rejections write nothing, so retrying never admits more than the limit. After `WithMaxCASAttempts` attempts (64 by default), or when the context ends,
the call stops: exhausting the attempts returns `ErrContention`, which is neither an admission nor an `ErrLimitExceeded` — the request was not
counted,
and the caller decides whether to fail open or closed. Through the budget and token-bucket limiter factories both limits are set under
`natsContention`
(`maxCASAttempts`, `maxCASBackoff`).

| Option               | Default | Description                                               |
|----------------------|---------|-----------------------------------------------------------|
| `WithMaxCASAttempts` | `64`    | Read-modify-write attempts per `Allow` call               |
| `WithMaxCASBackoff`  | `20ms`  | Ceiling of the randomized pause after a revision conflict |

## Bucket TTL

The bucket's key TTL (`WithMaxAge`) expires every key in the bucket, including keys written by other processes sharing it. When the bucket already
exists with a different key TTL, `New` returns `ErrBucketTTLMismatch` and leaves the bucket untouched instead of rewriting it under those processes.
Align the configuration, use another bucket, or pass `WithMigrateBucketTTL()` to update the existing bucket deliberately; through the budget and
token-bucket limiter factories the switch is `storage.nats.migrateBucketTTL: true`.

## Bucket storage

New buckets are file-backed. The server cannot change a bucket's storage type, so `New` returns `ErrBucketStorageMismatch` and leaves the bucket
untouched when it already exists with another one; align the configuration, recreate the bucket, or use another bucket.
