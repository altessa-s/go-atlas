# redis

```go
import "github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
```

Package `redis` implements token bucket storage using Redis with Lua scripts for atomic operations. Provides distributed rate limiting across multiple
service instances.

## Options

| Option          | Default        | Description                                                                         |
|-----------------|----------------|-------------------------------------------------------------------------------------|
| `WithKeyPrefix` | `ratelimit:`   | Prefix for Redis keys                                                               |
| `WithClock`     | `clock.Real()` | Request timestamps the windows are scored and pruned by (tests pass a `clock.Fake`) |

The scripts take the request time from the clock, so a fake clock moves the windows: advancing it past a period frees the slots without a
real wait. Key expiry (`PEXPIRE`, one period after the last request) still runs on the Redis server clock; it can only drop a window early
when the fake clock runs a period or more behind real time.

## Multi-key admission

`Provider` implements `storages.MultiStorage`. `AllowAll` runs one Lua script that prunes and counts every window first and adds the request to all of
them only when each admits, so a refusal takes no slot; windows keep the millisecond resolution of `Allow`. The single-key `Allow` script is
unchanged.

Give the keys a shared hash tag, such as `{tenant-1}:global` and `{tenant-1}:host:example.com`, or place the tag in the key prefix. For keys without a
common tag, only a raw `*redis.Client` can run the script, after `ROLE` and `INFO cluster` verify a single non-cluster primary. Each provider shares one
in-flight check and reuses success for 30 seconds on its configured clock. The next call after expiry checks again so the provider can detect failover.
Each caller can cancel its own wait. The shared check has a 15-second deadline, and a failed check is retried on the next call. Cluster, Ring and
decorated clients return a `*storages.KeyError` wrapping `ErrCrossSlot` before sending any commands.

| Error          | Description                                                                     |
|----------------|---------------------------------------------------------------------------------|
| `ErrCrossSlot` | Keys of one `AllowAll` call on an unverified or sharded client do not share a hash tag |
