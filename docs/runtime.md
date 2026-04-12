# Runtime

Key concepts for the `core/runtime` package tree — shutdown hooks, resource cleanup, and application metadata.

```
import "github.com/altessa-s/go-atlas/core/runtime"
import "github.com/altessa-s/go-atlas/core/runtime/appinfo"
```

---

## Package map

| Package | Purpose | Docs |
|---------|---------|------|
| `core/runtime` | Shutdown hooks, GC resource cleanup | this file |
| `core/runtime/appinfo` | Application metadata, version queries, env vars, directory paths | [appinfo.md](appinfo.md) |
| `core/runtime/concurrency` | Adaptive concurrency limits, batch processing | [concurrency.md](concurrency.md) |
| `core/runtime/panics` | Panic recovery and runtime assertions | [concurrency.md](concurrency.md) |
| `core/retry` | Retry loops with backoff and context cancellation (moved out of `core/runtime/retry`) | [concurrency.md](concurrency.md) |
| `core/runtime/signals` | OS signal handling with priority and worker pools | [signals.md](signals.md) |

All packages live in the `core/` layer: stdlib only, zero external dependencies.

---

## Shutdown hooks

`OnShutdown` registers a `ShutdownHook` (`func(ctx context.Context) error`) to run
during application shutdown. `RunShutdownHooks` executes all registered hooks.

### Ordering and guarantees

- **LIFO** — hooks execute in reverse registration order, so foundational resources
  registered first are cleaned up last.
- **At most once** — `RunShutdownHooks` is guarded by `sync.Once`; calling it from
  multiple goroutines is safe and hooks run exactly once.
- **Errors don't abort** — a failing hook does not prevent subsequent hooks from
  running. All errors are collected and returned via `errors.Join`.
- **Context deadline** — each hook receives the caller's context, so a timeout on
  the shutdown sequence is enforced by passing a context with a deadline.

### Example

```go
// Register in startup order — db first, cache second.
// They execute in reverse: cache flushes, then db closes.
runtime.OnShutdown(func(ctx context.Context) error {
    return db.Close()
})

runtime.OnShutdown(func(ctx context.Context) error {
    return cache.Flush(ctx)
})

// Trigger during shutdown (e.g. from a signal handler):
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := runtime.RunShutdownHooks(ctx); err != nil {
    logger.Error("shutdown errors", slog.Any("error", err))
}
```

### Integration with signals and panic recovery

A typical application wires shutdown hooks into the signal handler so that
`RunShutdownHooks` is called on `SIGTERM`/`SIGINT`. Combined with
`defer panics.Handle(ctx)` in spawned goroutines, this ensures resources are
released even when a goroutine panics. See [concurrency.md](concurrency.md) for
signal and panic handling details.

---

## Resource cleanup

Type-safe wrappers around Go 1.24's GC-triggered cleanup APIs.

### `AddCleanup`

Attaches a cleanup function to an object that runs after the object becomes
unreachable. The returned `Cleanup` handle can cancel the cleanup before it fires.

```go
cleanup := runtime.AddCleanup(conn, func(id string) {
    releaseExternalResource(id)
}, conn.ID())

// If the resource is released explicitly, cancel the GC cleanup:
cleanup.Stop()
```

`AddCleanup` is a thin generic wrapper around `runtime.AddCleanup` (Go 1.24+).
The cleanup function receives `arg` (not the object itself) and runs in a separate
goroutine.

### `ClearFinalizer`

Removes any finalizer previously set on an object via `runtime.SetFinalizer`.
Safe to call even if no finalizer was set.

```go
runtime.ClearFinalizer(obj)
```

### When to use cleanup vs shutdown hooks

| Use case | Mechanism |
|----------|-----------|
| Release resources tied to a specific object's lifetime (file handles, C memory) | `AddCleanup` |
| Release shared/global resources at application exit (database pools, flush buffers) | `OnShutdown` |

Cleanup functions are triggered by the garbage collector and may never run if the
process exits first. Shutdown hooks are explicit and run when `RunShutdownHooks` is
called. For critical resources, prefer shutdown hooks.

---

## Application info (`appinfo`)

The `appinfo` subpackage provides application metadata (name, version, commit),
version queries, environment variable helpers, and convention-based directory paths.

See [appinfo.md](appinfo.md) for the full reference.

---

## See also

- [appinfo.md](appinfo.md) — application metadata, version queries, env vars,
  directory paths
- [concurrency.md](concurrency.md) — batch processing, concurrency limits, retry,
  panic recovery
- [signals.md](signals.md) — OS signal handling, priority execution, worker pools,
  graceful shutdown
- [scheduler.md](scheduler.md) — periodic task scheduling
