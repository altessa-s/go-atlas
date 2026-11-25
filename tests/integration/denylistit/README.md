# denylistit

```go
import "github.com/altessa-s/go-atlas/tests/integration/denylistit"
```

These read-only integration tests check that a Redis replica rejects strict denylist reads and raw scans. Each test skips when its endpoint variables
are unset.

## Endpoints

| Test                      | Environment                                               | Endpoint                                     |
|---------------------------|-----------------------------------------------------------|----------------------------------------------|
| `TestDirectReplica`       | `REDIS_REPLICA_ADDR`                                      | Redis 7+ replica                             |
| `TestSentinelReplicaOnly` | `REDIS_SAFE_SENTINEL_ADDR`, `REDIS_SENTINEL_MASTER_NAME`   | Sentinel monitoring a primary with a replica |
