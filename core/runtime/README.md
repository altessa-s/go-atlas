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
| [rlimits](./rlimits)         | Linux process resource caps via setrlimit(2)                       |
| [seccomp](./seccomp)         | Linux seccomp-BPF denylist of ~22 dangerous syscalls               |
| [signals](./signals)         | OS signal handling with priority, rate limiting, graceful shutdown |

## Hardening sequence

The Linux security primitives in this tree (`nonewprivs`, `rlimits`,
`capabilities`, `seccomp`, `landlock`) are designed to be composed in a
canonical order. Apply them from cheapest/most-recoverable to most
irreversible:

```
nonewprivs → rlimits → capabilities → seccomp → landlock
```

Why this order:

1. **`nonewprivs`** runs first. It is the prerequisite for both
   `seccomp(2)` and `landlock_restrict_self(2)` on unprivileged
   callers (the alternative is `CAP_SYS_ADMIN`, which production
   hosts should not have).
2. **`rlimits`** runs next. `setrlimit(2)` is process-wide and
   side-effect-light: a misconfigured limit (rejected by validation)
   does not leave behind any partial state in the higher-cost steps
   below.
3. **`capabilities`** runs after rlimits because capability dropping
   is per-thread and irreversible — it should only run once the cheap
   passes are confirmed working.
4. **`seccomp`** runs after capabilities because the seccomp filter
   denylist may include capability-related syscalls (`unshare`,
   `setns`, etc.); installing the filter after capabilities means a
   later capability check cannot accidentally re-permit a denied
   syscall via privilege.
5. **`landlock`** runs last. The filesystem allowlist is the most
   irreversible step (no relaxation for the lifetime of the calling
   task) and depends on the `NO_NEW_PRIVS` set in step 1.

`core/plugins/sandbox.go` implements this sequence (minus seccomp,
which is intentionally not wired into the plugin sandbox so the
audited denylist stays stable across deployments — operators call
`seccomp.BlockDangerousSyscalls()` directly during host startup if
they want it).
