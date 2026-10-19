# retry

```go
import "github.com/altessa-s/go-atlas/core/retry"
```

Package `retry` provides a small, stdlib-only retry loop with context cancellation, configurable delay policies, and timer reuse — zero `time.After`
allocations per attempt, so it fits high-throughput retry loops in hot paths.

## Options

| Option               | Description                                              |
|----------------------|----------------------------------------------------------|
| `WithMaxAttempts`    | Max attempt index (0 = try once, -1 = unlimited)         |
| `WithMaxElapsedTime` | Wall-clock cap across all attempts                       |
| `WithShouldRetry`    | Predicate; `false` stops retrying immediately            |
| `WithNextDelay`      | Delay function `(attempt, err) -> Duration`; `<=0` stops |
| `WithOnRetry`        | Optional callback before each retry's sleep              |
| `WithSleep`          | Replaces the timer between attempts (fake clock in tests); its error ends `Do` |
| `WithStopBeforeDeadline` | Skip a retry whose delay would reach the context deadline; `Do` returns the last attempt's error, not `ctx.Err()` |

With no options `Do` calls `fn` exactly once: retrying needs both a non-zero `WithMaxAttempts` and a `WithNextDelay` policy.

## Deadlines and minimum delays

By default a retry whose backoff outlasts the context sleeps until the deadline and `Do` returns `ctx.Err()`. `WithStopBeforeDeadline()` checks the
remaining time first: when the next delay would reach the deadline no retry is started (and `WithOnRetry` is not called) and `Do` returns the error of
the last attempt, so callers keep the real failure class instead of a generic deadline error.

The `WithNextDelay` callback receives the failed attempt's error, so a minimum taken from the error — an HTTP `Retry-After`, a quota reset — is applied
there:

```go
retry.WithNextDelay(func(attempt int, err error) time.Duration {
    return max(backoff(attempt, err), retryAfterOf(err))
})
```

## Prebuilt policies

`Do(ctx, fn, opts...)` materializes its options on every call. Hot call paths (e.g. a per-request HTTP attempt) should build a `Policy` once and reuse
it — the options are processed a single time and each `Policy.Do` call skips option processing (a call that retries still allocates one reusable timer):

```go
var policy = retry.NewPolicy(retry.WithMaxAttempts(3), retry.WithNextDelay(retry.Exponential(cfg)))

err := policy.Do(ctx, attempt) // same semantics as retry.Do
```

A `Policy` is immutable after construction and safe for concurrent use.

## Exponential backoff

`Exponential(ExponentialConfig)` returns a `NextDelayFunc` computing `BaseDelay × Factor^attempt` (default factor 1.5), capped at `MaxDelay` and widened
by up to `Jitter` (0–1) of random extra delay.

`GetExponentialConfig` / `PutExponentialConfig` pool config structs for hot paths to avoid allocations in tight retry loops.
