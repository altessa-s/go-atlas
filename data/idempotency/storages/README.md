# storages

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages"
```

Package `storages` defines the `Storage` interface for idempotency key persistence. Implementations live in subpackages and are injected into
the top-level `idempotency.Keeper` to supply the underlying storage backend.

## Key types

| Type / Interface | Description                                                    |
|------------------|----------------------------------------------------------------|
| `Storage`        | Interface: AttemptLock, Complete, Delete                       |
| `Status`         | Key state: `StatusInProgress`, `StatusSuccess`                 |
| `State`          | Holds the current status and optional response data for a key  |

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [memory](./memory)     | In-memory backend with TTL         |
| [nats](./nats)         | NATS JetStream storage             |
| [redis](./redis)       | Distributed Redis storage          |
