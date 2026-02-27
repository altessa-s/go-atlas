# factory

```go
import "github.com/altessa-s/go-atlas/data/idempotency/factory"
```

Package `factory` builds `idempotency.Keeper` instances and their storage backends from configuration objects. Infrastructure references (Redis
client, NATS connection) are injected once at construction.
