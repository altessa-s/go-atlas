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

A script reaches one node, so on a `*redis.ClusterClient` or `*redis.Ring` every key of a call must hash alike: give the keys one hash tag, such as
`{tenant-1}:global` and `{tenant-1}:host:example.com`, or put a tag in the key prefix (`WithKeyPrefix("{app}:")`, which places every key of the
provider on one node). Redis hashes the text between the first `{` and the next `}` when it is not empty, and the whole key otherwise. A call whose
keys hash differently fails before any network I/O with a `*storages.KeyError` wrapping `ErrCrossSlot`, instead of a `CROSSSLOT` reply from the
server. Single-node and failover clients accept any keys.

| Error          | Description                                                                     |
|----------------|---------------------------------------------------------------------------------|
| `ErrCrossSlot` | Keys of one `AllowAll` call on a Cluster or Ring client do not share a hash tag |
