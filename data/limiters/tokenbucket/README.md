# tokenbucket

```go
import "github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
```

Package `tokenbucket` implements rule-based rate limiting using a sliding window algorithm. Supports IP/CIDR-based rules, client-specific limits,
and authentication token extraction with configurable storage backends and sophisticated rule matching that prioritizes more specific rules.

## Options

| Option                         | Default | Description                          |
|--------------------------------|---------|--------------------------------------|
| `WithIPCacheSize`              | 1000    | LRU cache capacity for IP lookup     |
| `WithExtractToken`             | noop    | Function to extract auth token       |
| `WithExtractClientIPAddress`   | noop    | Function to extract client IP        |
| `WithClientService`            | nil     | Client-specific rate limit service   |
| `WithLogger`                   | discard | Structured logger                    |

## Subpackages

| Package                                      | Description                     |
|----------------------------------------------|---------------------------------|
| [factory](./factory)                         | Configuration-based creation    |
| [storages/memory](./storages/memory)         | In-memory storage backend       |
| [storages/redis](./storages/redis)           | Distributed Redis storage       |
| [storages/nats](./storages/nats)             | NATS KV storage backend         |
