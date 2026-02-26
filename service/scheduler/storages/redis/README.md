# redis

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
```

Package `redis` implements `scheduler.Storage` using RedisJSON for document storage and RediSearch for indexed
querying. Safe for concurrent use across multiple processes, making it suitable for distributed scheduling where
multiple service instances share scheduler state through a single Redis cluster.

## Prerequisites

The Redis server must have the **RedisJSON** and **RediSearch** modules loaded.

## Usage

```go
storage := redis.New(client,
    redis.WithKeyPrefix("myapp:scheduler"),
    redis.WithHistoryTTL(7 * 24 * time.Hour),
)

// Create RediSearch indexes once at startup (idempotent).
if err := storage.EnsureIndexes(ctx); err != nil {
    return err
}

sched := scheduler.New(storage)
```

## Options

| Option                  | Default       | Description                                                              |
|-------------------------|---------------|--------------------------------------------------------------------------|
| `WithKeyPrefix`         | `scheduler`   | Prefix for all Redis keys, used to namespace multiple schedulers         |
| `WithHistoryTTL`        | 0 (no expiry) | TTL applied to history entry keys for automatic expiration by Redis     |
| `WithMaxHistoryPerTask` | 0 (unlimited) | Auto-trim oldest entries per task when the cap is exceeded              |

## Key patterns

```
{prefix}:task:{task_id}                    -- task state (JSON)
{prefix}:history:{task_id}:{history_id}    -- history entry (JSON)
{prefix}:idx:tasks                         -- RediSearch FT index
{prefix}:idx:history                       -- RediSearch FT index
```

## Filter support

CEL filter expressions are translated to RediSearch query syntax via the `redisearch` translator package and evaluated
server-side, leveraging RediSearch full-text indexes for efficient filtered paginated listing.
