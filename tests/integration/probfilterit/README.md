# probfilterit

```go
import "github.com/altessa-s/go-atlas/tests/integration/probfilterit"
```

This suite exercises the Bloom and Cuckoo Redis storages and their factories. See the [integration guide](../README.md) for Redis Stack setup.

## Topology tests

The topology tests read server state and skip when their endpoint variables are unset. Supply endpoints that accept the default Redis credentials.

| Variable                     | Required endpoint or value                                                  |
|------------------------------|-----------------------------------------------------------------------------|
| `REDIS_CLUSTER_NODE_ADDR`    | A reachable cluster node for the direct-client rejection case               |
| `REDIS_REPLICA_ADDR`         | A reachable replica for the direct-client rejection case                    |
| `REDIS_SAFE_CLUSTER_ADDR`    | A Redis Cluster seed with safe eviction policies on every primary           |
| `REDIS_SAFE_SENTINEL_ADDR`   | A Sentinel monitoring a primary with a safe eviction policy                 |
| `REDIS_SENTINEL_MASTER_NAME` | The monitored master name for the Sentinel test                             |
