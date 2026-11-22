# redis

```go
import "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
```

Package `redis` stores AIP-155 request ID entries in Redis.

## Options

| Option             | Default      | Purpose                                        |
|--------------------|--------------|------------------------------------------------|
| `WithCleanupGrace` | `1h`         | Extra retention for clock skew (minimum 1 ms). |
| `WithKeyPrefix`    | `requestid:` | Isolates this Guard's entries in Redis         |

Each Lua script operates on one hash. Expiry comparisons use the caller's millisecond timestamp. Completing a request installs the result's own
expiry. Redis reclaims the hash after the remaining caller-supplied duration plus the configured cleanup grace. Use a dedicated namespace and a no-eviction policy.
Set the grace above the maximum amount by which elapsed Redis time can exceed elapsed caller time, including a change of caller replica.
