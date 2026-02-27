# factory

```go
import "github.com/altessa-s/go-atlas/data/cache/factory"
```

Package `factory` builds `cache.Cache` instances from configuration objects. Infrastructure references (Redis client, etc.) are injected once at
construction and reused across every cache the factory creates.
