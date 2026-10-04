# redis

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
```

Package `redis` implements `scheduler.Storage` using RedisJSON for document storage and RediSearch for indexed querying. Safe for concurrent use across
multiple processes, so several service instances can share scheduler state through a single Redis cluster.

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
exactly one claim returns `1` and wins the run; the rest get `0` and skip. This prevents concurrent claims of the same active occurrence.

## Filter support

CEL filter expressions are translated to RediSearch query syntax via the `redisearch` translator package and evaluated server-side, leveraging
RediSearch full-text indexes for efficient filtered paginated listing.

## Atomic run finalization

`FinishRun` uses a server-side Lua script to compare `last_run_id` and the unfinished-run marker before updating execution fields. It rejects stale or
repeated completion, preserves current task configuration and paused/disabled status, and does not advance a concurrently changed schedule. Claiming and
finishing protect scheduler state; external task side effects still need idempotency when abandoned runs can be retried.
