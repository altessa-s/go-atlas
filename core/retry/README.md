# retry

```go
import "github.com/altessa-s/go-atlas/core/retry"
```

Package `retry` provides a small, stdlib-only retry loop with context cancellation, configurable delay policies, and timer reuse —
zero `time.After` allocations per attempt, making it suitable for high-throughput retry scenarios in hot paths.

## Config

| Field            | Description                                              |
|------------------|----------------------------------------------------------|
| `MaxAttempts`    | Max attempt index (0 = try once, -1 = unlimited)         |
| `MaxElapsedTime` | Wall-clock cap across all attempts                       |
| `ShouldRetry`    | Predicate; `false` stops retrying immediately            |
| `NextDelay`      | Delay function `(attempt, err) -> Duration`; `<=0` stops |
| `OnRetry`        | Optional callback after each failed attempt              |

## Exponential backoff

`GetExponentialConfig` / `PutExponentialConfig` pool config structs for hot paths to avoid allocations in tight retry loops.
