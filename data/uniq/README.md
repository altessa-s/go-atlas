# uniq

```go
import "github.com/altessa-s/go-atlas/data/uniq"
```

Package `uniq` provides unique value management with multiple storage backends (Redis, NATS, no-op). Supports adding, checking existence,
retrieving, and removing unique keys with optional associated values. Default TTL is 24 hours.

## Options

| Option           | Default | Description            |
|------------------|---------|------------------------|
| `WithSerializer` | JSON    | Serialization format   |

## Subpackages

| Package                              | Description                      |
|--------------------------------------|----------------------------------|
| [factory](./factory)                 | Configuration-based creation     |
| [providers/redis](./providers/redis) | Redis-backed provider            |
| [providers/nats](./providers/nats)   | NATS KV provider                 |
| [providers/noop](./providers/noop)   | No-op provider for testing       |
