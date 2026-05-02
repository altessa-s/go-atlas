# Signals

Key concepts for working with `core/runtime/signals` — OS signal handling with priority-based execution, worker pools, and graceful shutdown.

```
import "github.com/altessa-s/go-atlas/core/runtime/signals"
```

All operations are fully thread-safe. Registration, start/stop, and signal processing can be called concurrently without external synchronization.

---

## Quick start

```go
handler := signals.New(
    signals.WithSignals(syscall.SIGTERM, syscall.SIGINT),
    signals.WithWorkerPoolSize(20),
    signals.WithHandlerTimeout(10 * time.Second),
    signals.WithShutdownTimeout(30 * time.Second),
    signals.WithExecutionMode(signals.ParallelMode),
)

handler.AddHandlerWithPriority(func(ctx context.Context, sig os.Signal) error {
    return flushMetrics(ctx)
}, signals.PriorityHigh, syscall.SIGTERM)

handler.AddHandler(func(ctx context.Context, sig os.Signal) error {
    return closeConnections(ctx)
}, syscall.SIGTERM, syscall.SIGINT)

handler.Start()
handler.Wait() // blocks until shutdown completes
```

---

## Configuration

`signals.New` accepts functional options. All have sensible defaults:

| Option | Default | Description |
|--------|---------|-------------|
| `WithSignals(...)` | none | OS signals to listen for |
| `WithWorkerPoolSize(n)` | `10` | Max concurrent handler goroutines |
| `WithHandlerTimeout(d)` | `5s` | Per-handler execution timeout |
| `WithShutdownTimeout(d)` | `30s` | Max time for graceful shutdown |
| `WithExecutionMode(m)` | `SequentialMode` | Handler execution strategy |
| `WithErrorHandler(fn)` | `nil` | Callback for handler errors, panics, and timeouts |
| `WithSignalChannelBuffer(n)` | `1` | OS signal channel buffer size |

---

## Handler registration

### Signal-specific handlers

```go
handler.AddHandler(fn, syscall.SIGTERM, syscall.SIGINT)
```

Registers `fn` at `PriorityNormal` (50) for the listed signals. Multiple handlers can be registered for the same signal. Nil handlers are silently ignored.

### Priority handlers

```go
handler.AddHandlerWithPriority(fn, signals.PriorityHighest, syscall.SIGTERM)
```

Higher numeric priority executes first. In `SequentialMode`, a higher priority handler must complete before a lower priority one starts. In `ParallelMode`,
priority determines startup order but not completion order.

### Broadcast handlers

```go
handler.AddBroadcastHandler(fn)
handler.AddBroadcastHandlerWithPriority(fn, signals.PriorityHigh)
```

Broadcast handlers fire on every received signal regardless of signal type. They are merged with signal-specific handlers and sorted by priority before
execution.

### Method chaining

All registration methods return the `SignalHandler` interface, enabling chaining:

```go
signals.New(
    signals.WithSignals(syscall.SIGTERM, syscall.SIGINT),
).
    AddHandlerWithPriority(criticalShutdown, signals.PriorityHighest, syscall.SIGTERM).
    AddHandler(cleanup, syscall.SIGTERM, syscall.SIGINT).
    AddBroadcastHandler(logSignal).
    Start().
    Wait()
```

---

## Priority levels

Handlers execute in descending priority order (highest first):

| Constant | Value | Typical use |
|----------|-------|-------------|
| `PriorityHighest` | 100 | Critical system handlers, graceful shutdown coordinators |
| `PriorityHigh` | 75 | Important business-logic handlers |
| `PriorityNormal` | 50 | Default — general cleanup |
| `PriorityLow` | 25 | Background tasks, deferred cleanup |
| `PriorityLowest` | 1 | Logging, metrics collection |

Custom `Priority` values outside 1–100 are supported.

---

## Execution modes

### `SequentialMode` (default)

Handlers run one at a time in strict priority order within a single goroutine. Predictable ordering, suitable when handlers have dependencies on each other.

### `ParallelMode`

Handlers run concurrently in separate goroutines (up to the worker pool size). Priority determines startup order but completion order is non-deterministic.
When the worker pool is exhausted, handlers fall back to synchronous execution in the current goroutine to prevent goroutine explosion.

```go
signals.New(
    signals.WithExecutionMode(signals.ParallelMode),
    signals.WithWorkerPoolSize(20),
    signals.WithSignals(syscall.SIGTERM),
)
```

---

## Worker pool

The worker pool limits concurrent handler goroutines across all signals. When all slots are occupied:

- **In the signal listener** — the signal is dropped and the error handler is called
  with `"worker pool is full, signal handler dropped"`.
- **In parallel mode** — the handler executes synchronously in the current goroutine
  instead of spawning a new one.

Tune `WithWorkerPoolSize` based on how many handlers may run simultaneously. The default of 10 is suitable for most applications.

---

## Timeouts

### Handler timeout

Each handler receives a context derived from the global context with a deadline set to `WithHandlerTimeout`. If the handler does not complete in time, a
`*TimeoutError` is reported to the error handler. The handler's goroutine is not forcibly killed — it should respect `ctx.Done()`.

When `handlerTimeout` is zero or negative, the handler receives the global context without an additional deadline (fast path, no extra goroutine).

### Shutdown timeout

`Shutdown(ctx)` applies `WithShutdownTimeout` as a deadline on the provided context (via `corecontext.ApplyTimeout`, so an existing tighter deadline is
preserved). If in-flight handlers do not complete within this window, shutdown returns a wrapped context error.

---

## Error handling

Register a custom error handler to observe failures:

```go
signals.New(
    signals.WithErrorHandler(func(sig os.Signal, err error) {
        if signals.IsTimeout(err) {
            logger.Warn("handler timed out", slog.Any("signal", sig))
            return
        }
        logger.Error("handler failed", slog.Any("signal", sig), slog.Any("error", err))
    }),
    signals.WithSignals(syscall.SIGTERM),
)
```

### Error types

| Type | When | Check |
|------|------|-------|
| `*TimeoutError` | Handler exceeds `handlerTimeout` | `signals.IsTimeout(err)` |
| `*PanicError` | Handler panics | `errors.As(err, &pe)` — `.Panic` holds the recovered value |
| any `error` | Handler returns non-nil error | standard `errors.Is`/`errors.As` |

Panics within the error handler itself are recovered and silently discarded to prevent cascading failures.

---

## Lifecycle

### Start

```go
handler.Start()
```

Begins listening for OS signals via `signal.Notify` and dispatching to handlers. Idempotent — calling `Start` more than once has no effect. Returns the
handler for chaining.

### Wait

```go
handler.Wait()
```

Blocks until the signal handler stops. Typically called in `main` after `Start` to keep the process alive.

### Shutdown

```go
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
if err := handler.Shutdown(ctx); err != nil {
    logger.Error("shutdown error", slog.Any("error", err))
}
```

Graceful shutdown sequence:
1. Stops receiving new OS signals (`signal.Stop`).
2. Signals the background listener to exit.
3. Waits for all in-flight handlers to complete (or the deadline to expire).
4. Runs `runtime.RunShutdownHooks(ctx)` — integrating with the global shutdown hook
   system (see [README.md](README.md)).
5. Cancels the global handler context.

If the context expires before handlers finish, shutdown hooks are still attempted on a best-effort basis with `context.Background()`.

`Stop()` is a convenience wrapper that calls `Shutdown(context.Background())`.

Both `Shutdown` and `Stop` are idempotent — subsequent calls return nil.

---

## Integration with shutdown hooks

`Signal.Shutdown` automatically calls `runtime.RunShutdownHooks` after all signal handlers complete. This means resources registered via
`runtime.OnShutdown` are released as part of the shutdown sequence without additional wiring:

```go
// At startup:
runtime.OnShutdown(func(ctx context.Context) error {
    return db.Close()
})

// Signal handler triggers RunShutdownHooks automatically:
handler := signals.New(signals.WithSignals(syscall.SIGTERM))
handler.Start()
handler.Wait()
```

See [README.md](README.md) for shutdown hook ordering and guarantees.

---

## Performance

- **Priority sorting** — for ≤10 handlers, insertion sort (O(n²) but fast for small N);
  for 11–50 handlers, counting sort (O(n)); for >50 handlers, a bucket-based priority queue with O(1) insertion.
- **Zero-timeout fast path** — handlers without a timeout execute synchronously in the
  caller's goroutine with no extra goroutine or channel overhead.
- **Worker pool** — channel-based semaphore prevents goroutine explosion under heavy
  signal load.

### Tuning constants

| Constant | Value | Purpose |
|----------|-------|---------|
| `SmallHandlerCountThreshold` | 10 | Below this, insertion sort is used |
| `LargeHandlerCountThreshold` | 50 | Above this, priority queue is used |
| `HandlerSliceInitialCapacity` | 32 | Pre-allocated handler slice capacity |
| `PriorityBucketCount` | 5 | Number of priority queue buckets |
| `ResponsiveTimeoutDuration` | 50ms | Grace period for parallel handlers after shutdown |

---

## See also

- [README.md](README.md) — shutdown hooks, resource cleanup, `core/runtime` package map
- [concurrency.md](concurrency.md) — batch processing, concurrency limits, retry,
  panic recovery
