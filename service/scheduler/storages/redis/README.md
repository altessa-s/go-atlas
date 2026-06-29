# redis

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
```

Package `redis` implements `scheduler.Storage` using RedisJSON for document storage and RediSearch for
indexed querying. Safe for concurrent use across multiple processes, so several service instances
can share scheduler state through a single Redis cluster.

## Prerequisites

The Redis server must have the **RedisJSON** and **RediSearch** modules loaded.

## Options

| Option                  | Default       | Description                                                     |
|-------------------------|---------------|-----------------------------------------------------------------|
| `WithKeyPrefix`         | `scheduler`   | Prefix for all Redis keys, used to namespace multiple schedulers|
| `WithHistoryTTL`        | 0 (no expiry) | TTL applied to history entry keys for automatic expiration      |
| `WithMaxHistoryPerTask` | 0 (unlimited) | Auto-trim oldest entries per task when the cap is exceeded      |

## Atomic run claim

`ClaimRun` transitions a task `active → running` for a specific occurrence with a single server-side Lua `EVAL` script (read status / `next_run_at`,
check the fence, then `JSON.SET` the new state). Redis runs the script atomically under its single-threaded execution, so among concurrent schedulers
exactly one claim returns `1` and wins the run; the rest get `0` and skip. This makes duplicate execution impossible even during a leader-election
split-brain.

## Filter support

CEL filter expressions are translated to RediSearch query syntax via the `redisearch` translator
package and evaluated server-side, leveraging RediSearch full-text indexes for efficient filtered
paginated listing.
