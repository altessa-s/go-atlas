# redis

```go
import "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
```

Package `redis` implements idempotency storage using Redis. Safe for concurrent use across multiple processes, so several service
instances can share idempotency state.
