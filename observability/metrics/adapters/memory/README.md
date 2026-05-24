# memory

```go
import "github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
```

Package `memory` provides an in-memory metrics adapter implementation for testing.
Metric values are accumulated in goroutine-safe maps and exposed via lookup methods.

## Usage

```go
adapter := memory.NewAdapter()
collector := metrics.NewCollector(adapter, "test")

// Record metrics
collector.Counter("requests", "Total requests").
    WithLabels("method", "GET").
    Add(1)

// Assert in tests
value := adapter.GetCounter("test_requests", map[string]string{"method": "GET"})
require.Equal(t, 1.0, value)
```

## Key Types

| Type | Description |
|------|-------------|
| `Adapter` | In-memory metrics adapter for testing |

## Methods

| Method | Description |
|--------|-------------|
| `NewAdapter` | Creates a new in-memory adapter |
| `GetCounter` | Retrieves counter value by name and labels |
| `GetGauge` | Retrieves gauge value by name and labels |
| `GetHistogram` | Retrieves histogram observations by name and labels |
| `Reset` | Clears all stored metrics |
| `ListMetrics` | Returns all registered metric descriptors |

## Features

- Zero external dependencies
- Thread-safe metric accumulation
- Perfect for unit tests and benchmarks
- Supports all standard metric types (counter, gauge, histogram)
- Label matching for precise assertions