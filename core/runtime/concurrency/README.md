# concurrency

```go
import "github.com/altessa-s/go-atlas/core/runtime/concurrency"
```

Package `concurrency` provides adaptive concurrency limit calculation and a concurrent batch processor for parallelizing work across slices.

## Batch processing

| Function         | Description                                             |
|------------------|---------------------------------------------------------|
| `Process`        | Apply a function to every item with bounded concurrency |
| `ProcessCollect` | Same as `Process` but collect transformed results       |

## Options

| Option            | Description                                                  |
|-------------------|--------------------------------------------------------------|
| `WithConcurrency` | Fixed concurrency limit (number of worker goroutines)        |
| `WithLimitFunc`   | Dynamic limit via a `ConcurrencyLimitFunc` strategy          |
| `WithStopOnError` | Cancel remaining work after the first error                  |
| `WithOnSuccess`   | Callback invoked after each successfully processed item      |
| `WithOnError`     | Callback invoked with the item and error after each failure  |

## Concurrency strategies

| Factory                          | Strategy                            |
|----------------------------------|-------------------------------------|
| `ConcurrencyForEnvironment`      | Preset limits by workload profile   |
| `MemoryAwareConcurrency`         | Scale by available heap memory      |
| `LoadAwareConcurrency`           | Scale by system load average        |
| `AdaptiveConcurrency`            | Combine memory + load factors       |
| `ConnectionPoolAwareConcurrency` | Limit by available pool connections |
| `DefaultLimitFunc`               | `NumCPU * 2` (I/O-bound default)    |

## Environment presets

| Environment                    | Concurrency           |
|--------------------------------|-----------------------|
| `EnvironmentMemoryConstrained` | 1                     |
| `EnvironmentCPUBound`          | `NumCPU`              |
| `EnvironmentIOBound`           | `NumCPU * 2`          |
| `EnvironmentHighThroughput`    | `NumCPU * 4`          |
| `EnvironmentRateLimited`       | 3                     |
