# limiters

```go
import "github.com/altessa-s/go-atlas/data/limiters"
```

Package `limiters` provides shared types and interfaces for rate limiting used by both gRPC interceptors and HTTP middlewares. Defines the common
contract without transport-specific implementation details.

## Key types

| Type / Interface | Description                                           |
|------------------|-------------------------------------------------------|
| `Limiter`        | Interface: `Limit(ctx) (*LimitInfo, error)`           |
| `Func`           | Function adapter for `Limiter`                        |
| `LimitInfo`      | Rate limit status (Limit, Remaining, Reset)           |

## Errors

| Error              | Description                                 |
|--------------------|---------------------------------------------|
| `ErrLimitExceeded` | Returned when the rate limit is exceeded    |

## Subpackages

| Package                          | Description                              |
|----------------------------------|------------------------------------------|
| [tokenbucket](./tokenbucket)     | Token bucket rate limiting with rules    |
| [budget](./budget)               | Distributed budget limiter for outbound requests |
| [storages](./storages)           | Shared storage backends (memory, Redis, NATS) |
