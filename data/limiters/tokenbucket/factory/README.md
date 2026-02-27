# factory

```go
import "github.com/altessa-s/go-atlas/data/limiters/tokenbucket/factory"
```

Package `factory` builds `tokenbucket.RuleLimiter` instances and their storage backends from configuration objects. Infrastructure references
(Redis client, NATS connection) are injected once at construction.
