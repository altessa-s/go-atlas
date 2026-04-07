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
| [capabilities](./capabilities) | Linux capability dropping via capset(2) / prctl(2)               |
| [concurrency](./concurrency) | Adaptive concurrency limits, batch processing                      |
| [helpers](./helpers)         | Goroutine ID extraction (debug only)                               |
| [landlock](./landlock)       | Linux Landlock LSM filesystem allowlist (kernel 5.13+)             |
| [nonewprivs](./nonewprivs)   | `PR_SET_NO_NEW_PRIVS` primitive (SUID-escalation defense)          |
| [panics](./panics)           | Panic recovery, assertions (`Must`, `MustNonNil`, `MustError`)     |
| [retry](./retry)             | Retry loop with exponential backoff and context support            |
| [rlimits](./rlimits)         | Linux process resource caps via setrlimit(2)                       |
| [seccomp](./seccomp)         | Linux seccomp-BPF denylist of ~22 dangerous syscalls               |
| [signals](./signals)         | OS signal handling with priority, rate limiting, graceful shutdown |
