# redisutils

```go
import "github.com/altessa-s/go-atlas/internal/redis/redisutils"
```

Redis storage helpers build prefixed keys, store byte values and verify single-primary endpoints.

## Topology

`VerifyStandalonePrimary` accepts a client or pinned connection and requires the server to report a primary role and disabled cluster mode. Missing or conflicting evidence returns
`ErrNotStandalonePrimary`.
