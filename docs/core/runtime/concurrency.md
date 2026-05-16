# Concurrency

Key concepts for working with concurrency using `core/runtime` and its subpackages.

---

## Batch processing

`concurrency.Process` and `concurrency.ProcessCollect` run a function over a slice with bounded concurrency. They use a channel-based semaphore
internally and respect context cancellation.

```go
err := concurrency.Process(ctx, userIDs, func(ctx context.Context, id string) error {
    return syncUser(ctx, id)
},
    concurrency.WithConcurrency[string](8),
    concurrency.WithStopOnError[string](),
)
```

`ProcessCollect` preserves input order in the result slice by writing directly into a pre-allocated array indexed by position (no mutex, no sort):

```go
results, err := concurrency.ProcessCollect(ctx, urls, func(ctx context.Context, url string) (Response, error) {
    return httpGet(ctx, url)
},
    concurrency.WithConcurrency[string](4),
)
// results[i] corresponds to urls[i]
```

When concurrency resolves to 1 (or the slice has a single element), both functions fall back to sequential execution in the caller's goroutine — no
goroutine overhead.

### Error handling

- `WithStopOnError()` — cancels internal context on first error; in-flight items may
  still finish, but no new items start. Only the first error is returned.
- Without `WithStopOnError` — all items are processed; the first error is still returned.
- `WithOnSuccess` / `WithOnError` callbacks are invoked under a mutex, so shared state
  mutation is safe without external synchronization.

---

## Concurrency limits

The package provides several strategies for determining how many goroutines to run. All implement the `ConcurrencyLimitFunc` signature (`func() int`)
and are evaluated each time a limit decision is needed.

### Fixed by environment

```go
concurrency.ConcurrencyForEnvironment(concurrency.EnvironmentIOBound) // NumCPU * 2
```

| Environment | Workers |
|-------------|---------|
| `MemoryConstrained` | 1 |
| `CPUBound` | `NumCPU()` |
| `IOBound` | `NumCPU() * 2` (default) |
| `HighThroughput` | `NumCPU() * 4` |
| `RateLimited` | 3 |

### Memory-aware

Reads `runtime.MemStats` (cached with 1s TTL to avoid stop-the-world pauses) and scales concurrency by available memory:

```go
limitFn := concurrency.MemoryAwareConcurrency(
    100,  // < 100 MB free → 1 worker
    500,  // < 500 MB free → NumCPU workers
    1000, // < 1 GB free   → NumCPU*2 workers
)       //   ≥ 1 GB free  → NumCPU*4 workers
```

### Load-aware

Takes a function that returns the current system load (e.g. from `/proc/loadavg`):

```go
limitFn := concurrency.LoadAwareConcurrency(getLoad, 8.0, 4.0)
```

### Adaptive (multi-factor)

Combines memory and load pressure with configurable scaling factors:

```go
limitFn := concurrency.AdaptiveConcurrency(concurrency.AdaptiveConcurrencyConfig{
    MemoryLowThresholdMB:    100,
    MemoryMediumThresholdMB: 500,
    GetSystemLoad:           getLoad,
    HighLoadThreshold:       8.0,
})
```

### Connection pool-aware

Limits concurrency to available connections minus a reserve:

```go
limitFn := concurrency.ConnectionPoolAwareConcurrency(pool.Available, 2)
```

### Using limit functions with batch processing

Pass via `WithLimitFunc` (takes precedence over `WithConcurrency`):

```go
concurrency.Process(ctx, items, fn,
    concurrency.WithLimitFunc[Item](concurrency.MemoryAwareConcurrency(100, 500, 1000)),
    concurrency.WithStopOnError[Item](),
)
```

---

## Retry

`retry.Do` retries a function with configurable backoff, attempt limits, and context cancellation. It reuses a single `time.Timer` across attempts to
avoid allocations.

```go
err := retry.Do(ctx, retry.Config{
    MaxAttempts: 3,                    // attempts 0..3 (4 total calls)
    NextDelay:   retry.Exponential(retry.ExponentialConfig{
        BaseDelay: 500 * time.Millisecond,
        MaxDelay:  10 * time.Second,
        Factor:    1.5,
        Jitter:    0.2,
    }),
    ShouldRetry: func(err error) bool {
        return !errors.Is(err, ErrNotFound) // don't retry on not-found
    },
    OnRetry: func(attempt int, err error, delay time.Duration) {
        logger.Warn("retrying", slog.Int("attempt", attempt), slog.Any("error", err))
    },
}, func(ctx context.Context) error {
    return callExternalAPI(ctx)
})
```

### Stop conditions

`Do` returns when any of these occurs:
- `fn` returns nil (success)
- Context is canceled (`ctx.Err()`)
- `MaxAttempts` exhausted
- `MaxElapsedTime` exceeded
- `ShouldRetry` returns false
- `NextDelay` is nil or returns <= 0

### ExponentialConfig pooling

For hot paths, use `GetExponentialConfig` / `PutExponentialConfig` to reuse config objects from a `sync.Pool`:

```go
cfg := retry.GetExponentialConfig()
defer retry.PutExponentialConfig(cfg)
cfg.BaseDelay = 100 * time.Millisecond
cfg.MaxDelay = 5 * time.Second
```

---

## Panic recovery

`panics.Handle` recovers panics in goroutines and routes them through configurable handlers. All configuration uses atomic operations and is
thread-safe.

```go
go func() {
    defer panics.Handle(ctx)
    riskyWork()
}()
```

### Assertions

Assertion helpers convert contract violations into panics with clear messages:

```go
panics.MustNonNil(cfg, "config is required")
panics.MustNonZero(timeout, "timeout must be set")
panics.Must(port > 0, "port must be positive")

val := panics.MustResult(parseConfig(path)) // panics on error, returns value
```

### Global handlers

Register handlers that run on every recovered panic across the application:

```go
panics.AddGlobalPanicHandler(func(ctx context.Context, r any) {
    metrics.IncrCounter("panics_total", 1)
})
```

### Configuration

- `SetReallyPanic(true)` — re-panic after handling (useful in tests)
- `SetLogger(logger)` — custom logger for the default handler
- `SetLoggerFromContext(fn)` — extract logger from context per-call

---

## Shutdown hooks

`runtime.OnShutdown` registers cleanup functions executed in LIFO order when the application terminates. Hooks run at most once (via `sync.Once`)
regardless of how many callers invoke `RunShutdownHooks`.

```go
runtime.OnShutdown(func(ctx context.Context) error {
    return db.Close()
})

runtime.OnShutdown(func(ctx context.Context) error {
    return cache.Flush(ctx)
})

// Later, during shutdown (cache flushes first, then db closes):
if err := runtime.RunShutdownHooks(ctx); err != nil {
    logger.Error("shutdown errors", slog.Any("error", err))
}
```

Errors from individual hooks do not prevent subsequent hooks from running — all errors are collected and returned via `errors.Join`.

---

## Signal handling

`signals.Signal` provides OS signal handling with priority-based execution, worker pools, and rate limiting.

```go
handler := signals.New(
    signals.WithSignals(syscall.SIGTERM, syscall.SIGINT),
    signals.WithWorkerPoolSize(20),
    signals.WithHandlerTimeout(10 * time.Second),
    signals.WithShutdownTimeout(30 * time.Second),
    signals.WithExecutionMode(signals.ParallelMode),
)

handler.AddHandlerWithPriority(func(_ context.Context, sig os.Signal) error {
    return flushMetrics()
}, signals.PriorityHigh, syscall.SIGTERM)

handler.AddHandler(func(_ context.Context, sig os.Signal) error {
    return closeConnections()
}, syscall.SIGTERM, syscall.SIGINT)

handler.Start()
handler.Wait()
```

### Priority levels

Handlers execute by priority (highest first). Predefined levels:

| Constant | Value |
|----------|-------|
| `PriorityLowest` | 1 |
| `PriorityLow` | 25 |
| `PriorityNormal` | 50 (default) |
| `PriorityHigh` | 75 |
| `PriorityHighest` | 100 |

### Execution modes

- `SequentialMode` (default) — handlers run one at a time within each priority level
- `ParallelMode` — independent handlers run concurrently (up to worker pool size)

### Broadcast handlers

`AddBroadcastHandler` registers a handler that fires on any signal, not just specific ones.

---

## Resource cleanup

Type-safe wrappers around Go 1.24's `runtime.AddCleanup` for GC-triggered resource release:

```go
cleanup := runtime.AddCleanup(obj, func(id string) {
    releaseExternalResource(id)
}, resourceID)

// Cancel cleanup if resource is released explicitly:
cleanup.Stop()
```

---

## Patterns and guidelines

### Prefer `Process` over manual goroutine management

Instead of writing `sync.WaitGroup` + semaphore + error collection boilerplate, use `concurrency.Process`. It handles cancellation, error propagation,
callbacks, and sequential fallback automatically. When concurrency resolves to 1 (or the slice has a single element), both `Process` and
`ProcessCollect` fall back to sequential execution in the caller's goroutine — no goroutine overhead.

### Choose the right concurrency limit

- **I/O-bound** (HTTP calls, database queries): `EnvironmentIOBound` or `MemoryAwareConcurrency`
- **CPU-bound** (hashing, compression): `EnvironmentCPUBound`
- **External APIs with rate limits**: `EnvironmentRateLimited` or `ConnectionPoolAwareConcurrency`
- **Variable load**: `AdaptiveConcurrency` with both memory and load thresholds

### Always use `defer panics.Handle(ctx)` in spawned goroutines

Unrecovered panics in goroutines crash the entire process — Go does not recover panics across goroutine boundaries. `panics.Handle` logs the panic,
runs registered handlers, and prevents process termination (unless `SetReallyPanic(true)` is configured).

### Register shutdown hooks early

`OnShutdown` uses LIFO order. Register foundational resources (database, message broker) first so they are cleaned up last, after higher-level
components that depend on them.

### Use `retry.Do` for transient failures

Wrap external calls (HTTP, gRPC, database reconnects) in `retry.Do` with exponential backoff and jitter. Always provide `ShouldRetry` to skip retries on
permanent failures (e.g. not-found, validation errors, authentication failures).

### Prevent concurrent execution with `atomic.Bool` guards

Use `CompareAndSwap` to ensure only one goroutine executes a periodic cycle at a time. This is lighter than a mutex when the goal is to skip
overlapping invocations rather than queue them:

```go
if !o.dispatchRunning.CompareAndSwap(false, true) {
    return nil // previous cycle still running, skip
}
defer o.dispatchRunning.Store(false)
```

This pattern is used throughout the codebase for scheduler cycles: outbox dispatch, health checks, secret rotation, OPA policy updates, and cursor
cleanup.

### Use `context.WithoutCancel` for cleanup operations

When a store update or cleanup must complete even if the caller's context is canceled, derive a detached context with `context.WithoutCancel`:

```go
updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), o.updateTimeout)
defer cancel()
if err := o.store.UpdateEvents(updateCtx, processedEvents...); err != nil {
    // handle error
}
```

This is critical for outbox event status updates and graceful HTTP shutdown — a canceled parent context must not prevent the operation from finishing.

### Use `corecontext.ApplyTimeout` / `WithMaxTimeout`

Avoid stacking timeouts. `ApplyTimeout` only adds a timeout when the context does not already carry a deadline — if one exists, it returns the
context unchanged:

```go
ctx, cancel := corectx.ApplyTimeout(ctx, 5*time.Second)
defer cancel()
```

`WithMaxTimeout` caps the deadline at a maximum duration. If the existing deadline is sooner, the context is returned unchanged; if it is later (or
absent), a new deadline is set:

```go
ctx, cancel := corectx.WithMaxTimeout(ctx, 30*time.Second)
defer cancel()
```

Both functions return a no-op cancel func when no new context is created, so `defer cancel()` is always safe.

### Use `singleflight` for request deduplication

When multiple goroutines may request the same resource concurrently (cache miss, secret fetch, database metadata), use `singleflight.Group` to
coalesce calls and prevent cache stampede:

```go
result, err, _ := t.fetchGroup.Do(key, func() (any, error) {
    return t.updateValueWithRetry(ctx, key)
})
```

Only the first caller executes the function; all others block and receive the same result. This pattern is used in the cache, secrets manager, and
Mongo client.

### Reuse timers in loops with `coretime.TimerStopAndDrain`

Avoid `time.After` in loops — each call allocates a new timer that is not garbage collected until it fires. Instead, create one `time.Timer` and
reset it each iteration. Before calling `Reset`, drain the channel to prevent stale events:

```go
timer := time.NewTimer(interval)
defer coretime.TimerStopAndDrain(timer)

for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-timer.C:
        doWork()
        coretime.TimerStopAndDrain(timer)
        timer.Reset(interval)
    }
}
```

`TimerStopAndDrain` uses a non-blocking select so it never blocks even if another goroutine has already consumed the event.

### Safe resource release under panic

When a resource (e.g. a distributed lock) must be released whether the function returns normally or panics, use a mutex + bool guard callable from
both `defer` and the panic handler:

```go
var lockReleased bool
var lockMutex sync.Mutex

safeRelease := func() {
    lockMutex.Lock()
    defer lockMutex.Unlock()
    if !lockReleased {
        _ = lk.Release(context.Background())
        lockReleased = true
    }
}

defer panics.HandleWithOpts(ctx,
    panics.NewHandleOpts().SetReallyPanic(false),
    func(ctx context.Context, r any) { safeRelease() },
)

err = fn(ctx)
safeRelease()
```

The guard ensures the lock is released exactly once regardless of control flow.

### Use `StopOnError: false` for non-critical fan-out

When checking multiple independent resources (health checks, leader election callbacks, secret warmup), set `StopOnError: false` so all items are
processed even when some fail:

```go
err := concurrency.Process(ctx, checks, runCheck,
    concurrency.WithConcurrency[Check](4),
    // No WithStopOnError — check all services even if one is unhealthy.
)
```

The first error is still returned, but every item gets its chance to execute.

### Use atomics for concurrent state flags

Prefer `atomic.Bool` for running-state flags, `atomic.Int64` for counters, and `atomic.Int32` for gauges. They are cheaper than mutexes for single-value
access and make concurrent access explicit:

```go
type Coordinator struct {
    activeWatchers    atomic.Int32 // gauge: current subscription count
    listCallsInFlight atomic.Int32 // gauge: in-progress list calls
}

type Outbox struct {
    eventsInFlight atomic.Int64 // counter: events being dispatched
}
```

Increment with `Add(1)`, decrement with `Add(-1)`, and read with `Load()`. For boolean flags that gate execution, see the `atomic.Bool` guard pattern
above.
