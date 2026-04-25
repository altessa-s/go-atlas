# redis

```go
import "github.com/altessa-s/go-atlas/data/limiters/storages/redis"
```

Package `redis` implements token bucket storage using Redis with Lua scripts for atomic operations. Provides distributed rate limiting across
multiple service instances.
