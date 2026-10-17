# sampler

```go
import "github.com/altessa-s/go-atlas/observability/tracing/sampler"
```

Package `sampler` provides sampling strategies for distributed tracing. Samplers decide whether a trace should be recorded and exported.

## Built-in samplers

| Sampler                  | Description                                                  |
|--------------------------|--------------------------------------------------------------|
| `AlwaysOn`               | Sample every trace                                           |
| `AlwaysOff`              | Drop every trace                                             |
| `NewTraceIDRatio(ratio)` | Deterministic percentage based on trace ID                   |
| `NewParentBased(root, opts...)` | Defer to parent's sampling decision; use `root` for root spans; `With*Parent*` options override per parent kind |
