# retry

```go
import "github.com/altessa-s/go-atlas/core/retry"
```

Package `retry` provides a small, stdlib-only retry loop with context cancellation, configurable delay policies, and timer reuse —
zero `time.After` allocations per attempt, so it fits high-throughput retry loops in hot paths.

## Options

| Option               | Description                                              |
|----------------------|----------------------------------------------------------|
| `WithMaxAttempts`    | Max attempt index (0 = try once, -1 = unlimited)         |
| `WithMaxElapsedTime` | Wall-clock cap across all attempts                       |
| `WithShouldRetry`    | Predicate; `false` stops retrying immediately            |
| `WithNextDelay`      | Delay function `(attempt, err) -> Duration`; `<=0` stops |
| `WithOnRetry`        | Optional callback after each failed attempt              |

## Exponential backoff

`GetExponentialConfig` / `PutExponentialConfig` pool config structs for hot paths to avoid allocations in tight retry loops.
