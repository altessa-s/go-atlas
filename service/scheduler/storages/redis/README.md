# redis

```go
import "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
```

Package `redis` implements `scheduler.Storage` using RedisJSON for document storage and RediSearch for
indexed querying. Safe for concurrent use across multiple processes, making it suitable for distributed
scheduling where multiple service instances share scheduler state through a single Redis cluster.

## Prerequisites

The Redis server must have the **RedisJSON** and **RediSearch** modules loaded.

## Options

| Option                  | Default       | Description                                                     |
|-------------------------|---------------|-----------------------------------------------------------------|
| `WithKeyPrefix`         | `scheduler`   | Prefix for all Redis keys, used to namespace multiple schedulers|
| `WithHistoryTTL`        | 0 (no expiry) | TTL applied to history entry keys for automatic expiration      |
| `WithMaxHistoryPerTask` | 0 (unlimited) | Auto-trim oldest entries per task when the cap is exceeded      |

## Filter support

CEL filter expressions are translated to RediSearch query syntax via the `redisearch` translator
package and evaluated server-side, leveraging RediSearch full-text indexes for efficient filtered
paginated listing.
