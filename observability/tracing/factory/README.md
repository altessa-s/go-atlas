# factory

```go
import "github.com/altessa-s/go-atlas/observability/tracing/factory"
```

Package `factory` provides configuration-based creation of `tracing.Tracer` instances. Reads from `config.Tracing` to select the adapter,
sampler, service name, and other settings needed for production-ready distributed tracing.
