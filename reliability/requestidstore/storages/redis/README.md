# redis

```go
import "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
```

Package `redis` stores AIP-155 request ID entries in Redis.

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithKeyPrefix` | `requestid:` | Isolates this Guard's entries in Redis |

Each Lua script operates on one hash. Expiry comparisons use the caller's millisecond timestamp. Completing a request installs the result's own
expiry. Redis reclaims the hash after the remaining caller-supplied duration plus one minute. Use a dedicated namespace and a no-eviction policy.
Physical TTL assumes the caller clock advances with elapsed time within the grace period.
