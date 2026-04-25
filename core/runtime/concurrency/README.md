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

Configure via functional options: `WithConcurrency`, `WithLimitFunc`, `WithStopOnError`, `WithOnSuccess`, `WithOnError`.

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
