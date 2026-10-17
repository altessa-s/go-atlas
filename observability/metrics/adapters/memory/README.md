# memory

```go
import "github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
```

Package `memory` provides an in-memory metrics adapter implementation for testing. Metric values are accumulated in goroutine-safe maps and exposed via
lookup methods.

## Usage

```go
adapter := memory.New()
collector := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("test"))

// Record metrics
collector.MustCounter(metrics.MetricOpts{Name: "requests", Help: "Total requests", LabelNames: []string{"method"}}).
    WithLabels(metrics.Labels{"method": "GET"}).
    Add(1)

// Assert in tests
value := adapter.CounterValue("test_requests", map[string]string{"method": "GET"})
require.Equal(t, 1.0, value)
```

## Key Types

| Type | Description |
|------|-------------|
| `Adapter` | In-memory metrics adapter for testing |

## Methods

| Method | Description |
|--------|-------------|
| `New` | Creates a new in-memory adapter |
| `CounterValue` | Returns the counter sum by name and labels |
| `GaugeValue` | Returns the latest gauge value by name and labels |
| `HistogramCount` | Returns the number of histogram observations by name and labels |
| `HistogramSum` | Returns the sum of histogram observations by name and labels |
| `Exists` | Reports whether a metric with the given name was registered |
| `Names` | Returns the names of all registered metrics |

## Features

- Zero external dependencies
- Thread-safe metric accumulation
- Perfect for unit tests and benchmarks
- Supports all standard metric types (counter, gauge, histogram)
- Label matching for precise assertions
