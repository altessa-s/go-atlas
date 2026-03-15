# stats

```go
import "github.com/altessa-s/go-atlas/data/probfilter/stats"
```

Package `stats` defines statistics types for probabilistic filter operations (item count, capacity, false-positive rate, memory usage).

## FilterStats fields

| Field               | Type        | Description                                                    |
|---------------------|-------------|----------------------------------------------------------------|
| `Capacity`          | `int64`     | Maximum number of items the filter can hold                    |
| `ItemCount`         | `int64`     | Approximate number of items currently stored                   |
| `FillRatio`         | `float64`   | Ratio of used capacity (0.0–1.0)                               |
| `FalsePositiveRate` | `float64`   | Estimated false-positive probability                           |
| `MemoryUsageBytes`  | `int64`     | Memory used by the filter in bytes (may be zero for remote storage) |
| `LastRebuild`       | `time.Time` | Time of the last rebuild (only for `RebuildableFilter`)        |
| `StorageType`       | `string`    | Storage backend identifier (e.g., `"memory"`, `"redis"`)      |
