# runtime

```go
import "github.com/altessa-s/go-atlas/core/runtime"
```

Package `runtime` provides low-level runtime utilities: GC cleanup hooks, finalizer management, and a global shutdown hook registry.

## Functions

| Function           | Description                                                     |
|--------------------|-----------------------------------------------------------------|
| `AddCleanup`       | Attach a GC cleanup callback to an object (`runtime.AddCleanup` wrapper) |
| `ClearFinalizer`   | Remove a finalizer previously set via `runtime.SetFinalizer`    |
| `OnShutdown`       | Register a shutdown hook (LIFO order)                           |
| `RunShutdownHooks` | Execute all registered hooks exactly once; errors are joined    |

## Subpackages

| Package                      | Description                                                        |
|------------------------------|--------------------------------------------------------------------|
| [appinfo](./appinfo)         | Application metadata, build info, env vars, directory paths        |
| [concurrency](./concurrency) | Adaptive concurrency limits, batch processing                      |
| [helpers](./helpers)         | Goroutine ID extraction (debug only)                               |
| [panics](./panics)           | Panic recovery, assertions (`Must`, `MustNonNil`, `MustError`)     |
| [retry](./retry)             | Retry loop with exponential backoff and context support            |
| [signals](./signals)         | OS signal handling with priority, rate limiting, graceful shutdown |
