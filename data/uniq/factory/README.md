# factory

```go
import "github.com/altessa-s/go-atlas/data/uniq/factory"
```

Package `factory` builds `uniq.Uniq` instances from configuration objects. Infrastructure references (Redis client, NATS connection) are injected
once at construction.
