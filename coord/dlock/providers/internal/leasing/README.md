# leasing

```go
import "github.com/altessa-s/go-atlas/coord/dlock/providers/internal/leasing"
```

Shared lease lifecycle for the [MongoDB](../../mongo) and [SQL](../../sqldb) lock providers. Stores supply conditional lease operations;
`Engine` handles acquisition, renewal, loss notification, and release.

## Key types

| Type       | Purpose                                           |
|------------|---------------------------------------------------|
| `Store`    | Conditional storage operations and health checks  |
| `Engine`   | Lease acquisition and lifecycle management         |
| `Settings` | Resolved provider timing and logger               |

## Renewal deadlines

Lease deadlines are measured from the last confirmed acquisition or renewal attempt's start. Failed renewals retry after half the remaining
lease, with a 1ms floor, bounded by the next regular attempt and the confirmed deadline. Each call is bounded by the smaller of the operations
timeout and the remaining lease. At the deadline, `Lost()` closes and the engine conditionally releases the unconfirmed lease; a late success
cannot revive it.
