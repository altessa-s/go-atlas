# rlimits

```go
import "github.com/altessa-s/go-atlas/core/runtime/rlimits"
```

A small, dependency-free Go wrapper around the Linux `setrlimit(2)` syscall
for one-shot startup-time process hardening. Each configured rlimit pins
both soft and hard limits to the configured value, so the soft limit can
never be raised back above it for the lifetime of the process.

## API

| Symbol | Purpose |
|---|---|
| `Apply(opts ...Option)` | Installs every configured rlimit; **irreversible** for the hard-limit direction |
| `WithMemoryBytes(n int64)` | Caps `RLIMIT_AS` (virtual address space); 0 = unset |
| `WithMaxOpenFiles(n int64)` | Caps `RLIMIT_NOFILE`; 0 = unset |
| `WithMaxProcesses(n int64)` | Caps `RLIMIT_NPROC` (per real UID, **not** per process — see Caveats); 0 = unset |
| `WithMaxFileSizeBytes(n int64)` | Caps `RLIMIT_FSIZE`; 0 = unset |
| `WithDisableCoreDumps()` | Pins `RLIMIT_CORE` to 0 (prevents post-crash memory leaks) |
| `ErrUnsupported` | Platform doesn't support setrlimit (non-Linux) |
| `ErrFailed` | The setrlimit syscall failed (sentinel wrapping the kernel errno via `%w`) |
| `ErrInvalidOption` | Operator supplied a negative rlimit value |

`Apply` follows the functional-options convention used elsewhere in go-atlas
(e.g. `core/io/files.Walk`, `core/plugins.NewManager`, `core/runtime/landlock`).
Negative values are rejected before any syscall runs.

## Quick start

```go
package main

import (
    "log"

    "github.com/altessa-s/go-atlas/core/runtime/rlimits"
)

func main() {
    err := rlimits.Apply(
        rlimits.WithMemoryBytes(512 << 20),  // 512 MiB
        rlimits.WithMaxOpenFiles(4096),
        rlimits.WithMaxProcesses(256),
        rlimits.WithDisableCoreDumps(),
    )
    if err != nil {
        log.Fatalf("rlimits: %v", err)
    }
    run()
}
```

## When to use it

- **Long-running services** that want hard caps on runaway allocations
  or descriptor leaks.
- **CLI tools processing untrusted input** (archive extractors, template
  renderers) where malformed input could trigger pathological resource
  use.
- **Plugin hosts** (see `core/plugins/sandbox.go`) that compose rlimits
  with `core/runtime/nonewprivs` and `core/runtime/landlock`.
- **Services handling sensitive data** that must not leak memory
  contents via core dumps — `WithDisableCoreDumps()` is cheap and
  effective.

## Caveats

- **Linux only.** On non-Linux platforms `Apply` returns `ErrUnsupported`
  so cross-platform code can import the package without build tags.
  setrlimit(2) is a POSIX syscall, but rlimit resource sets and
  semantics differ subtly across kernels (darwin's `RLIMIT_NPROC` is
  not quite Linux's) and we do not currently ship a BSD/darwin
  implementation.
- **Process-wide.** Every goroutine, every child process, every
  in-process database driver or connection pool shares the same
  resource pool. Tune `WithMaxOpenFiles` with care — it counts
  sockets, files, pipes, epoll, timerfd, signalfd, etc.
- **`RLIMIT_NPROC` is per real UID, not per process.** The kernel counts
  every process owned by the same real UID as the calling process,
  including unrelated processes started by other binaries running under
  the same user account. On a host running multiple instances of the
  service under the same UID, or running as a shared system user
  (`nobody`, `daemon`), setting `WithMaxProcesses` low enough to bound
  this service can starve unrelated processes — and the symptom
  (`fork: Resource temporarily unavailable` from a sibling process) is
  hard to trace back to this rlimit. Run the service under a dedicated
  UID, or leave `MaxProcesses` unset and bound process count via
  systemd `TasksMax=` / cgroup `pids.max`, which are per-cgroup rather
  than per-UID.
- **Irreversible for hard-limit reductions.** Once a hard limit is
  lowered, an unprivileged process cannot raise it.
- **Partial application on error.** If `Apply` fails midway through the
  rlimit sequence, earlier successful rlimits remain in effect. This is
  intentional — a half-applied sandbox is better than silently dropping
  requested restrictions.
- **Zero = unset.** A zero value leaves the kernel default in place for
  that resource. The one exception is `WithDisableCoreDumps`, which is
  a boolean flag and actively pins `RLIMIT_CORE` to 0 when enabled.
- **Defense in depth, not isolation.** A compromised dependency already
  loaded in-process can still allocate up to the configured cap and read
  any file the process already holds open.

## Testing

Because `setrlimit` on hard limits is irreversible and process-wide, a
unit test that actually calls `Apply` would poison every subsequent test
in the same binary. The test suite therefore only exercises the
validation layer (`validateOptions`) and the option accumulation
behavior. For real syscall coverage, run your integration tests in a
dedicated subprocess or container.

## Hardening sequence

This package is **step 2** in the standard go-atlas hardening
sequence:

`nonewprivs` → `rlimits` → `capabilities` → `seccomp` → `landlock`

`rlimits` runs after `nonewprivs` (the cheapest no-prerequisite step)
and before `capabilities` because `setrlimit` is process-wide and
side-effect-light, so a misconfigured rlimit (rejected by validation)
does not leave behind a half-applied capability state.

## See also

- [`setrlimit(2)` man page](https://man7.org/linux/man-pages/man2/setrlimit.2.html)
- `core/runtime/nonewprivs` — `PR_SET_NO_NEW_PRIVS` primitive (step 1)
- `core/runtime/capabilities` — Linux capability dropping (step 3)
- `core/runtime/seccomp` — syscall denylist via seccomp-BPF (step 4)
- `core/runtime/landlock` — filesystem allowlist (step 5)
- `core/plugins/sandbox.go` — real-world consumer composing the sequence
