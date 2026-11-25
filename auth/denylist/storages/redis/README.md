# redis

```go
import redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
```

Redis-backed revocation store for the [denylist](../../) subsystem. A revocation is a Redis string at `keyPrefix+key`. The caller owns the Redis
client and closes it after all store consumers have stopped.

## Options

| Option                   | Default             | Description                                                        |
|--------------------------|---------------------|--------------------------------------------------------------------|
| `WithKeyPrefix(string)`  | `denylist:revoked:` | Literal namespace prepended to every stored key.                   |
| `WithStrictExactReads()` | Off                 | Require an uncached client and verify the primary on every lookup. |

## Methods

| Method                              | Description                                                                                         |
|-------------------------------------|-----------------------------------------------------------------------------------------------------|
| `New(client, opts...) (*Store, error)` | Borrow a Redis client. Strict mode validates the client configuration at construction.             |
| `IsRevoked(ctx, key) (bool, error)` | Check key existence. Strict mode checks the role and key in one keyed `EVAL_RO`.                    |
| `Revoke(ctx, key) error`             | Deny permanently, including keys previously revoked for a bounded time.                             |
| `RevokeUntil(ctx, key, ttl) error`   | Atomically extend retention. Round positive TTLs up to milliseconds. A non-positive TTL is a no-op. |
| `Restore(ctx, key) error`            | Remove a permanent or bounded revocation.                                                           |
| `RawSource() mirror.Source`          | Stream bare revoked keys for a mirror refresh.                                                      |
| `DataLoader() probfilter.DataLoader` | Stream the same bare keys for a filter rebuild. Count returns −1 (unknown).                         |

## Consistency

[negcache](../../negcache/) is an eventually consistent negative cache. A filter miss can admit a revoked token until the next successful rebuild.
Failed or skipped rebuilds extend that window. For exact answers, call `Store.IsRevoked` directly on an uncached primary. Durability across failover
depends on the deployment's replication guarantees.

Strict reads require Redis 7 or later and permission for `EVAL_RO`, `INFO replication`, and `EXISTS`. Construction accepts a standalone client or a
primary-routed cluster client with the default node factory. It rejects rings, unknown wrappers, replica routing on clusters, and client-side caching.
Each strict read bypasses the client cache and requires the serving Redis instance to report `role:master`, including when using Sentinel.

## Rebuilds

Both sources require a raw `*redis.Client` connected to a standalone primary. Each iteration borrows one connection, verifies `ROLE`, disabled cluster
mode and the server's `run_id`, then rechecks each SCAN page before yielding it. A topology or process change ends the stream with an error. Concurrent
writes follow Redis SCAN semantics. The prefix is matched literally and stripped from each key.

## Errors

| Error                      | Meaning                                                                                           |
|----------------------------|---------------------------------------------------------------------------------------------------|
| `ErrUnsupportedClient`      | Unsupported strict client configuration or a scan source that could not verify a stable primary. |
| `ErrReplicaReadNotAllowed`  | Strict cluster routing allows replicas, or the read script could not verify the primary role.    |

Operation errors have credential-free messages and support `errors.Is` matching against their causes.

## Usage

```go
store, err := redisstore.New(client, redisstore.WithStrictExactReads())
if err != nil {
    return err
}
if err := store.RevokeUntil(ctx, jti, time.Until(tokenExp)+jwt.DefaultLeeway); err != nil {
    return err
}
revoked, err := store.IsRevoked(ctx, jti)
```
