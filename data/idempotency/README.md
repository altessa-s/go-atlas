# idempotency

```go
import "github.com/altessa-s/go-atlas/data/idempotency"
```

Package `idempotency` provides duplicate request detection using idempotency keys. Supports multiple storage backends (memory, Redis, NATS) for
single-instance and distributed systems.

## Options

| Option           | Default | Description                  |
|------------------|---------|------------------------------|
| `WithLogger`     | discard | Structured logger            |
| `WithSerializer` | JSON    | Serialization format         |

## Subpackages

| Package                                  | Description                          |
|------------------------------------------|--------------------------------------|
| [factory](./factory)                     | Configuration-based creation         |
| [storages/memory](./storages/memory)     | In-memory backend with TTL           |
| [storages/redis](./storages/redis)       | Distributed Redis storage            |
| [storages/nats](./storages/nats)         | NATS JetStream storage               |
