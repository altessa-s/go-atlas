# redis

```go
import "github.com/altessa-s/go-atlas/data/cache/storages/redis"
```

Package `redis` implements the cache provider interface using Redis for distributed caching. Supports key prefixing for namespace isolation and
configurable TTL per entry.

`GetMany` implements `storages.MultiGetter` with a pipeline of `GET` commands (not `MGET`), so keys in different Redis Cluster slots work; the storage
prefix is applied and the result is keyed by the unprefixed keys.
