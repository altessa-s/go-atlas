# redis

```go
import redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
```

Redis-backed distributed exact revocation store for the [denylist](../../) subsystem. Each revoked key is a single Redis string
(`keyPrefix+key` → `"1"`); Redis expires bounded revocations natively via per-key TTL, so no background sweeper is needed.

A `Store` satisfies the [negcache](../../negcache/) `Authoritative` interface structurally (via `IsRevoked`) — it deliberately does not
import `negcache` — and implements [`probfilter.DataLoader`](../../../../data/probfilter/), so it can be both the authoritative tier
behind a negative cache and the source that rebuilds the cache's filter.

## Options

| Option                     | Default               | Description                              |
|----------------------------|-----------------------|------------------------------------------|
| `WithKeyPrefix(string)`    | `denylist:revoked:`   | Namespace prepended to every stored key. |

## Key types

| Type    | Description                                                                                          |
|---------|------------------------------------------------------------------------------------------------------|
| `Store` | Distributed revocation store over a `redis.UniversalClient`. Safe for concurrent use.                |

## Methods

| Method                                                      | Description                                                              |
|------------------------------------------------------------|--------------------------------------------------------------------------|
| `New(client, opts...) *Store`                              | Construct over a required `redis.UniversalClient`.                        |
| `IsRevoked(ctx, key) (bool, error)`                        | Authoritative revocation check (`EXISTS`).                               |
| `Revoke(ctx, key) error`                                   | Deny permanently (`SET key "1"`, no expiry).                            |
| `RevokeUntil(ctx, key, ttl) error`                         | Deny for `ttl`; non-positive `ttl` is a no-op.                          |
| `Restore(ctx, key) error`                                  | Remove the key (`DEL`).                                                  |
| `StreamValues(ctx) iter.Seq2[string, error]`               | SCAN all revoked keys, yield bare (unprefixed) keys.                    |
| `Count(ctx) (int64, error)`                                | Returns `-1` (unknown); an exact count needs a full SCAN.               |

## Usage

```go
client := goredis.NewClient(&goredis.Options{Addr: addr})
store := redisstore.New(client)

// Revoke a token by its jti until its expiry plus the verifiers' leeway.
_ = store.RevokeUntil(ctx, "jti-123", time.Until(tokenExp)+jwt.DefaultLeeway)

revoked, err := store.IsRevoked(ctx, "jti-123")

// Feed a negative cache rebuild (Store is a probfilter.DataLoader). Rebuild is not atomic: keep reads on the
// store while it runs and after a failure — see docs/auth/denylist.md#correctness-and-staleness.
if err := cache.Rebuild(ctx, store); err != nil {
    return err // do not serve IsRevoked from the cache
}
```
