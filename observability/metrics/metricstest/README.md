# metricstest

```go
import "github.com/altessa-s/go-atlas/observability/metrics/metricstest"
```

Test kit for [`observability/metrics`](../): it an in-memory metrics collector and value readers for tests that assert on emitted metrics.

## Key types

| Symbol | Description |
|--------|-------------|
| `NewTestCollector()` | In-memory collector backed by the memory metrics adapter |
| `GetCounterValue / GetGaugeValue / GetHistogramCount` | Read a metric value by name and label pairs |
| `GatherMetric` | Reports whether a metric was recorded |
