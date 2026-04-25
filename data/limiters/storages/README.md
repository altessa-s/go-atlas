# storages

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages"
```

Package `storages` defines the `Storage` interface for rate limiting backends. Implementations live in subpackages and are injected
into limiters (token bucket, budget) to supply the underlying rate tracking mechanism.

## Key types

| Type / Interface   | Description                                                      |
|--------------------|------------------------------------------------------------------|
| `Storage`          | Interface: Allow, Reset                                          |
| `LimitInfo`        | Rate limit state with Remaining count and Reset timestamp        |
| `ErrLimitExceeded` | Sentinel error returned when the request rate exceeds the limit  |

## Subpackages

| Package                | Description                        |
|------------------------|------------------------------------|
| [memory](./memory)     | In-memory backend with TTL         |
| [nats](./nats)         | NATS JetStream storage             |
| [redis](./redis)       | Distributed Redis storage          |
