# retry

```go
import "github.com/altessa-s/go-atlas/core/runtime/retry"
```

Package `retry` provides a small, stdlib-only retry loop with context cancellation, configurable delay policies, and timer reuse 
(zero `time.After` allocations).

## Usage

```go
cfg := retry.Config{
    MaxAttempts: 3,
    ShouldRetry: func(err error) bool { return true },
    NextDelay:   retry.Exponential(retry.ExponentialConfig{BaseDelay: 500 * time.Millisecond}),
}
err := retry.Do(ctx, cfg, func(ctx context.Context) error {
    return doThing(ctx)
})
```

## Config

| Field            | Description                                              |
|------------------|----------------------------------------------------------|
| `MaxAttempts`    | Max attempt index (0 = try once, -1 = unlimited)         |
| `MaxElapsedTime` | Wall-clock cap across all attempts                       |
| `ShouldRetry`    | Predicate; `false` stops retrying immediately            |
| `NextDelay`      | Delay function `(attempt, err) -> Duration`; `<=0` stops |
| `OnRetry`        | Optional callback after each failed attempt              |

## Exponential backoff

```go
retry.Exponential(retry.ExponentialConfig{
    BaseDelay: 100 * time.Millisecond,
    MaxDelay:  5 * time.Second,
    Factor:    1.5, // default
})
```

`GetExponentialConfig` / `PutExponentialConfig` pool config structs for hot paths.
