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
| `WithOnRetry`        | Optional callback after each failed attempt              |

With no options `Do` calls `fn` exactly once: retrying needs both a non-zero `WithMaxAttempts` and a `WithNextDelay` policy.

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
