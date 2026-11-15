# nats

```go
import "github.com/altessa-s/go-atlas/store/mongo/storages/nats"
```

Package `nats` implements cursor storage using NATS JetStream key-value store for MongoDB pagination state persistence.

## Expiration

Cursors are never deleted: the key TTL of the bucket is their only cleanup. `New(ctx, kv)` reads the bucket status and fails with
`coreerrs.ErrInvalidArgument` for a bucket without a positive key TTL, which would otherwise grow by one entry per issued page forever. Create the
bucket with a TTL — `DefaultTTL` (1h) matches the default of the Redis and in-memory cursor storages — or set one on an existing bucket before
handing it to `New`.

## Keys

NATS KeyValue accepts only `[-/_=.a-zA-Z0-9]` in a key, with no leading or trailing dot and no empty token, so cursor keys are escaped before they
reach
the bucket: every other byte, `=` itself, and every dot that would leave an empty token become `=HH` (upper-case hex). The key `users:page:2` is
stored
as `users=3Apage=3A2`. A key that is already valid and contains no `=` is stored under itself, so existing data stays reachable. A key containing `=`
now maps to another physical key than the raw one earlier releases wrote; migrate such entries or start on a fresh bucket. The escaped key may be at
most 3072 bytes (an escaped byte takes three); a longer one fails with `ErrKeyTooLong` before any network I/O.
