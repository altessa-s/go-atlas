# factory

```go
import "github.com/altessa-s/go-atlas/observability/health/factory"
```

Package `factory` provides configuration-based creation of `health.Coordinator` instances. Reads from `config.Health` to set cache TTL,
check timeout, shard count, and other coordinator options for production-ready health monitoring.
