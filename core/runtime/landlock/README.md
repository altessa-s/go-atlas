# landlock

```go
import "github.com/altessa-s/go-atlas/core/runtime/landlock"
```

A minimal, dependency-free Go wrapper around the Linux Landlock LSM
unprivileged filesystem sandbox (kernel 5.13+). Once `Apply` returns, the
calling process is confined to a strict filesystem allowlist for the rest
of its lifetime — even code reached via `dlopen` or spawned as a child
inherits the restrictions.

Landlock requires **no root, no capabilities, no setuid**. Any unprivileged
process can restrict itself; `Apply` automatically sets the
`PR_SET_NO_NEW_PRIVS` prerequisite as its first step (internally delegated
to [`core/runtime/nonewprivs`](../nonewprivs/README.md)), so callers do
not need to invoke `prctl` themselves.

## API

| Symbol | Purpose |
|---|---|
| `Apply(opts ...Option)` | Installs the ruleset; **irreversible** for the process |
| `WithReadPaths(...)` | Appends read+execute paths to the ruleset |
| `WithReadWritePaths(...)` | Appends read+execute+write+truncate paths |
| `Supported()` | `true` when the kernel supports Landlock v1+ |
| `ABIVersion()` | Highest Landlock ABI the kernel supports (1..6) |
| `ErrUnsupported` | Kernel doesn't support Landlock (sentinel) |
| `ErrFailed` | Ruleset setup failed (sentinel wrapping the kernel errno via `%w` — `errors.As` recovers the raw `syscall.Errno`) |
| `ErrInvalidOption` | Operator supplied an empty or non-absolute path (sentinel) |

`Apply` follows the functional-options convention used elsewhere in go-atlas
(e.g. `core/io/files.Walk`, `plugins.NewManager`, `data/probfilter.NewManager`).
Every path must be a non-empty absolute filesystem path; `Apply` validates the
allowlist before invoking any syscall, so misconfiguration surfaces as a plain
error (not wrapped in `ErrFailed` or `ErrUnsupported`).

## Quick start

```go
package main

import (
    "log"

    "github.com/altessa-s/go-atlas/core/runtime/landlock"
)

func main() {
    // Apply sets PR_SET_NO_NEW_PRIVS automatically as its first step, then
    // installs the ruleset. On failure the process stays unrestricted (the
    // NO_NEW_PRIVS bit, if successfully set, remains in effect); on success
    // every subsequent filesystem access outside the allowlist returns
    // EACCES.
    // The canonical glibc-host allowlist used by both the package
    // doc.go and ExampleApply. Adapt /etc/myservice and the
    // /var/{lib,log}/myservice entries to your host's directory layout.
    err := landlock.Apply(
        landlock.WithReadPaths(
            "/etc/myservice",    // read-only config
            "/lib",              // dynamic loader + libc
            "/lib64",            // dynamic loader + libc (multilib)
            "/usr/lib",          // shared libraries
            "/usr/lib64",        // shared libraries (multilib)
        ),
        landlock.WithReadWritePaths(
            "/var/lib/myservice",
            "/var/log/myservice",
        ),
    )
    if err != nil {
        log.Fatalf("landlock: %v", err)
    }

    // From this point onward the process can only read, execute, or
    // write the listed paths.
    run()
}
```

### Prerequisites — handled for you

`landlock_restrict_self(2)` requires either `CAP_SYS_ADMIN` or
`PR_SET_NO_NEW_PRIVS` on the calling thread. `Apply` first probes
kernel support via a side-effect-free `ABIVersion()` call; if Landlock
is unsupported it returns `ErrUnsupported` and **`NO_NEW_PRIVS` is not
set** — callers that fall back to "log and continue" are not silently
committed to NNP. Only after Landlock support is confirmed does `Apply`
set `NO_NEW_PRIVS` on the calling thread, then proceed with ruleset
construction. The `prctl` is idempotent when already set and, like the
Landlock domain itself, irreversible for the rest of the thread's
lifetime — calling `Apply` on a Landlock-capable kernel commits the
calling thread to both restrictions. If you specifically need to
preserve SUID-exec capability post-Landlock (a `CAP_SYS_ADMIN`
scenario), drop down to the raw syscalls in `golang.org/x/sys/unix`
rather than using this package.

### Multiple calls accumulate

Each `WithReadPaths` / `WithReadWritePaths` call appends to the running
allowlist rather than replacing it. This lets library code contribute path
requirements without coordinating with the top-level caller:

```go
baseOpts := []landlock.Option{
    landlock.WithReadPaths("/etc/myservice"),
    landlock.WithReadWritePaths("/var/lib/myservice"),
}

extra := loadLibraryPathRequirements()  // returns []landlock.Option

err := landlock.Apply(append(baseOpts, extra...)...)
```

## Use cases

| Scenario | What Landlock adds |
|---|---|
| Host-level hardening at service startup | Service can only read `/etc/myservice` + write its log/data dirs |
| Audit log writer | Write access limited to the audit directory even if the code is compromised |
| Secret loader | Read access limited to the credentials path + cache |
| Template renderer processing untrusted templates | Template can only touch its working directory |
| Plugin manager (see `plugins/sandbox.go`) | Plugins can't escape their allowlisted filesystem view |
| Test harness verifying the code-under-test doesn't touch unexpected paths | Fail-fast assertion via `EACCES` on forbidden access |

## Kernel ABI versions

| ABI | Kernel | Adds |
|-----|--------|------|
| 1   | 5.13   | read_file, read_dir, write_file, execute, remove_*, make_* |
| 2   | 5.19   | refer (cross-path rename/link) |
| 3   | 6.2    | truncate |
| 4   | 6.7    | ioctl_dev |
| 5   | 6.10   | scoped signals |
| 6   | 6.12   | additional restrictions |

This package negotiates the highest supported version at runtime and uses
flags up to ABI 3. On older kernels (v1, v2) the truncate bit is silently
dropped from the write mask — an acceptable degradation that keeps `Apply`
working on RHEL 9 and similarly-aged hosts.

## Caveats

- **Linux 5.13+ only.** On non-Linux platforms every function returns
  `ErrUnsupported` so cross-platform code can import the package without
  build tags.
- **Per-task (per-thread) scope, NOT process-wide in pure Go.**
  `landlock_restrict_self(2)` is per-task on Linux. The Go runtime has
  already created several OS threads (sysmon, GC, netpoll, GOMAXPROCS
  workers) by the time user code runs, and Go does not expose any way
  to iterate or pin every existing thread. `Apply` only restricts the
  goroutine's current OS thread; peer threads keep their original
  unrestricted Landlock domain. New tasks created via `clone(2)`
  inherit the parent task's domain — so a goroutine the scheduler later
  places on a peer thread can still touch paths outside the allowlist.
  For real process-wide restriction, apply Landlock externally (e.g. a
  C launcher that calls `landlock_restrict_self` before `execve(2)` of
  the Go binary) or use a Linux container runtime with the appropriate
  filesystem view. The in-process `Apply` call remains a useful
  defense-in-depth layer for the threads it can reach.
- **Irreversible per thread.** No "unsandbox" syscall. Misconfigured
  limits require a process restart to clear.
- **Defense in depth, not isolation.** Landlock restricts filesystem
  access only. It does nothing about network, signals, process creation,
  or memory — a malicious binary already loaded in-process can still read
  and corrupt host memory.
- **Symlinks: the kernel checks the resolved target, not the link
  itself.** A symlink that lives inside the allowlist but points to a
  target outside it returns `EACCES` on access of the target — there is
  no Landlock concept of "rejecting the symlink". Conversely, a symlink
  outside the allowlist that resolves to a path inside it grants access
  to the target. Always reason about the access in terms of resolved
  inodes, not link locations.
- **Strict allowlist.** Nothing is auto-added. The caller lists every
  path the process needs, including the dynamic loader and libc.
- **`NO_NEW_PRIVS` is set on the calling thread when Landlock is
  supported.** `Apply` first probes Landlock support via a
  side-effect-free `ABIVersion()` call; if Landlock is unsupported it
  returns `ErrUnsupported` and NNP is not set. On supported kernels,
  `Apply` sets `PR_SET_NO_NEW_PRIVS` on the calling thread before
  ruleset construction. If a later step fails (e.g. a listed path does
  not exist and `add_rule` returns `ENOENT`), `Apply` returns an
  `ErrFailed` with the `NO_NEW_PRIVS` bit still in effect on that
  thread. Operators should treat a successful `Apply` (or any failure
  on a Landlock-capable kernel) as a commitment to NNP for the calling
  thread's lifetime.

## Testing

Because `Apply` is irreversible and process-wide, a unit test that
actually calls it would restrict every subsequent test in the same binary
(and many would fail). The test suite therefore only exercises the
validation layer (`validateOptions`) and the option accumulation behavior.
For real syscall coverage, run your integration tests in a dedicated
subprocess or container.

## Hardening sequence

This package is the **final step** in the standard go-atlas hardening
sequence:

`nonewprivs` → `rlimits` → `capabilities` → `seccomp` → `landlock`

Landlock runs last because it is the most irreversible step (the
ruleset cannot be relaxed for the lifetime of the calling task) and
because the other primitives need to be in place before the
filesystem allowlist takes effect — `landlock_restrict_self(2)`
requires `PR_SET_NO_NEW_PRIVS`, and a typical hardening pipeline drops
capabilities (which removes `CAP_SYS_ADMIN`, the only alternative
prerequisite) before reaching here.

## See also

- [Kernel documentation](https://docs.kernel.org/userspace-api/landlock.html)
- [LWN article](https://lwn.net/Articles/859908/) (announcement, Landlock v1)
- `core/runtime/nonewprivs` — `PR_SET_NO_NEW_PRIVS` primitive (step 1)
- `core/runtime/rlimits` — process resource limits (step 2)
- `core/runtime/capabilities` — Linux capability dropping (step 3)
- `core/runtime/seccomp` — syscall denylist via seccomp-BPF (step 4)
- `plugins/sandbox.go` in this repository — a real-world consumer that
  adds plugin-specific conveniences (auto-include plugin dir, system libs)
  on top of the raw `landlock.Apply` call.
