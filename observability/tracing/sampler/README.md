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
| `NewParentBased(fallback)` | Defer to parent's sampling decision; use fallback for roots |

## Usage

```go
// Sample 10% of traces, respect parent decisions
s := sampler.NewParentBased(sampler.NewTraceIDRatio(0.1))

provider := tracing.New(tracing.WithSampler(s))
```
