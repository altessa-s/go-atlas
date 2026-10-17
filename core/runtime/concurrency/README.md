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

A positive `WithLimitFunc` result takes precedence; otherwise a positive `WithConcurrency` value is used, and with neither the limit falls back to
`DefaultLimitFunc`.

## Concurrency strategies

| Factory                          | Strategy                                                    |
|----------------------------------|-------------------------------------------------------------|
| `ConcurrencyForEnvironment`      | Preset limits by workload profile                           |
| `MemoryAwareConcurrency`         | Scale by remaining memory budget                            |
| `LoadAwareConcurrency`           | Scale by system load average                                |
| `AdaptiveConcurrency`            | Combine memory + load factors (`AdaptiveConcurrencyConfig`) |
| `ConnectionPoolAwareConcurrency` | Limit by available pool connections                         |
| `DefaultLimitFunc`               | `NumCPU * 2` (I/O-bound default)                            |

## Memory budgets

`MemoryAwareConcurrency` and `AdaptiveConcurrency` use the remaining Go runtime budget configured through `GOMEMLIMIT` or `debug.SetMemoryLimit`. The
remaining bytes are `max(0, limit - (MemStats.Sys - MemStats.HeapReleased))`; statistics are cached for `DefaultMemStatsCacheTTL` (one second) — tune
with `SetMemStatsCacheTTL`, force a refresh with `InvalidateMemStatsCache`. This measures runtime headroom, not free machine RAM. Without a finite
budget, the memory strategy uses `DefaultLimitFunc` and adaptive mode skips memory scaling.

Pass `WithAvailableMemory(source)` to either strategy to supply a thread-safe `MemorySource` for a container or host budget. It returns available bytes
and whether the budget is known. Returning `(0, true)` means exhausted; `(0, false)` means unknown. Callbacks should cache expensive probes.

## Environment presets

| Environment                    | Concurrency           |
|--------------------------------|-----------------------|
| `EnvironmentMemoryConstrained` | 1                     |
| `EnvironmentCPUBound`          | `NumCPU`              |
| `EnvironmentIOBound`           | `NumCPU * 2`          |
| `EnvironmentHighThroughput`    | `NumCPU * 4`          |
| `EnvironmentRateLimited`       | 3                     |
