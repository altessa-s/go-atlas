# Runtime security

Linux process-hardening primitives under `core/runtime/`. Five small, focused packages that lock down a Go process at startup so a compromised plugin or
dependency cannot escalate beyond the surface the operator granted.

```go
import (
    "github.com/altessa-s/go-atlas/core/runtime/capabilities"
    "github.com/altessa-s/go-atlas/core/runtime/landlock"
    "github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
    "github.com/altessa-s/go-atlas/core/runtime/rlimits"
    "github.com/altessa-s/go-atlas/core/runtime/seccomp"
)
```

> **Defense in depth, not isolation.** These primitives reduce blast
> radius. They do **not** replace a container, VM, or hypervisor: a
> malicious in-process dependency can still read every byte of the host's
> address space. Pair these with systemd, a container runtime, or a
> microVM for true isolation.

> **Linux only.** Every package returns `ErrUnsupported` on non-Linux
> platforms (and `seccomp` also returns it on Linux architectures other
> than amd64/arm64), so the same hardening code can compile and run on
> developer macOS workstations and Linux production hosts alike. It
> degrades to a no-op where the kernel cannot back it.

---

## Overview

| Package        | Mechanism                                          | Reversible | Scope          |
|----------------|----------------------------------------------------|------------|----------------|
| `nonewprivs`   | `prctl(PR_SET_NO_NEW_PRIVS)`: block SUID exec      | No         | Per-thread     |
| `rlimits`      | `setrlimit(2)`: cap memory, FDs, processes, etc.   | No (hard)  | Process-wide   |
| `capabilities` | `capset(2)` / bounding / ambient                   | No (drops) | Per-thread     |
| `seccomp`      | `seccomp(SECCOMP_SET_MODE_FILTER, TSYNC)` denylist | No         | Thread group   |
| `landlock`     | `landlock_restrict_self(2)` filesystem allowlist   | No         | Per-thread     |

### Platform support

| Package        | Minimum kernel | Architectures   | Build tags                          |
|----------------|----------------|-----------------|-------------------------------------|
| `nonewprivs`   | Linux 3.5      | any             | `linux` / `!linux`                  |
| `rlimits`      | Linux (any)    | any             | `linux` / `!linux`                  |
| `capabilities` | Linux (any)    | any             | `linux` / `!linux`                  |
| `seccomp`      | Linux 3.17     | amd64, arm64    | `linux && (amd64 \|\| arm64)`       |
| `landlock`     | Linux 5.13     | any             | `linux` / `!linux`                  |

`landlock` negotiates the highest mutually supported Landlock ABI at runtime: ABI 1 (Linux 5.13) for the base read/write/execute mask, ABI 3 (Linux
6.2) for the `truncate` flag. Older kernels in the 5.13 to 6.1 range silently lose `truncate`; everything else continues to work.

---

## Recommended hardening sequence

Apply primitives in the order `nonewprivs` → `rlimits` → `capabilities` → `seccomp` → `landlock`, once at process startup, before spawning
goroutines or loading plugins. The ordering matters:

- `nonewprivs` first because both `seccomp` and `landlock` require
  `PR_SET_NO_NEW_PRIVS` for unprivileged processes (and both will set it for you, but explicit ordering keeps the failure modes legible).
- `rlimits` next because hard-limit reductions are irreversible and you
  want them in place before any subsequent step can fail and abort startup.
- `capabilities` before `seccomp` so the privileged caps you actually
  need (e.g. `CAP_NET_BIND_SERVICE` for binding port 80) are still in effect during the privileged work, and dropped cleanly afterwards.
- `seccomp` before `landlock` because the seccomp denylist closes
  syscalls (`bpf`, `unshare`, `setns`, `keyctl`) that could otherwise be used to undermine the filesystem allowlist.

```go
package main

import (
    "errors"
    "log"

    "github.com/altessa-s/go-atlas/core/runtime/capabilities"
    "github.com/altessa-s/go-atlas/core/runtime/landlock"
    "github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
    "github.com/altessa-s/go-atlas/core/runtime/rlimits"
    "github.com/altessa-s/go-atlas/core/runtime/seccomp"
)

func harden() {
    // 1. NO_NEW_PRIVS — block SUID escalation across any future exec.
    if err := nonewprivs.Set(); err != nil && !errors.Is(err, nonewprivs.ErrUnsupported) {
        log.Fatalf("nonewprivs: %v", err)
    }

    // 2. Resource caps — irreversible for unprivileged processes.
    if err := rlimits.Apply(
        rlimits.WithMemoryBytes(512<<20),     // 512 MiB virtual address space
        rlimits.WithMaxOpenFiles(4096),
        rlimits.WithMaxProcesses(256),
        rlimits.WithDisableCoreDumps(),       // no post-crash secret leaks
    ); err != nil && !errors.Is(err, rlimits.ErrUnsupported) {
        log.Fatalf("rlimits: %v", err)
    }

    // 3. Capabilities — drop everything except what this binary needs.
    if err := capabilities.DropAllExcept(
        capabilities.CAP_NET_BIND_SERVICE,
    ); err != nil && !errors.Is(err, capabilities.ErrUnsupported) {
        log.Fatalf("capabilities: %v", err)
    }

    // 4. Seccomp denylist — close mount/kexec/ptrace/bpf/...
    if err := seccomp.BlockDangerousSyscalls(); err != nil && !errors.Is(err, seccomp.ErrUnsupported) {
        log.Fatalf("seccomp: %v", err)
    }

    // 5. Landlock — strict filesystem allowlist (must include libc/loader).
    if err := landlock.Apply(
        landlock.WithReadPaths(
            "/etc/myservice",
            "/lib", "/lib64",
            "/usr/lib", "/usr/lib64",
        ),
        landlock.WithReadWritePaths("/var/log/myservice"),
    ); err != nil && !errors.Is(err, landlock.ErrUnsupported) {
        log.Fatalf("landlock: %v", err)
    }
}

func main() {
    harden()
    // ... start servers, load plugins, etc.
}
```

> **Per-thread scope caveat.** `nonewprivs`, `capabilities`, and
> `landlock` operate on the calling thread, not the whole process.
> The Go runtime has typically already created several OS threads
> (sysmon, GC, netpoll, GOMAXPROCS workers) by the time `main` runs.
> An in-process call only restricts the goroutine's current thread; the
> scheduler may later place a goroutine on a peer thread that still
> holds the original (unrestricted) state. `seccomp` is the exception:
> it uses `SECCOMP_FILTER_FLAG_TSYNC` to propagate the filter to
> every thread in the thread group. For a real process-wide guarantee
> on the other primitives, apply them externally before the Go binary
> starts: a systemd unit (`NoNewPrivileges=`, `CapabilityBoundingSet=`,
> `LimitNOFILE=`), a container runtime (`--cap-drop`,
> `--security-opt=no-new-privileges`), or a small C launcher that calls
> the appropriate `prctl`/`capset`/`landlock_restrict_self` before
> `execve(2)` of the Go binary.

---

## `nonewprivs`

```go
import "github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
```

Sets the Linux `PR_SET_NO_NEW_PRIVS` bit. Once set, the calling thread and any binary it later `exec`s cannot gain privileges via `SUID`/`SGID`; the
kernel silently drops the ambient escalation. The bit is irreversible per thread and idempotent.

| Symbol            | Kind     | Purpose                                              |
|-------------------|----------|------------------------------------------------------|
| `Set()`           | function | Install `PR_SET_NO_NEW_PRIVS` on the calling thread  |
| `Enabled()`       | function | Report whether the bit is currently set              |
| `ErrUnsupported`  | sentinel | Non-Linux, or Linux < 3.5                            |
| `ErrFailed`       | sentinel | Wraps the underlying `prctl(2)` errno                |

```go
if err := nonewprivs.Set(); err != nil {
    log.Fatalf("nonewprivs: %v", err)
}
```

> **Breaks SUID tooling.** Enabling NNP breaks any in-process attempt to
> exec `sudo`, `su`, `mount`, `ping` (on distros that ship it SUID), and
> similar SUID binaries. Only enable this if the process has no need to
> spawn setuid helpers.

> **Prerequisite for the others.** `seccomp.BlockDangerousSyscalls` and
> `landlock.Apply` both call `nonewprivs.Set` internally before installing
> their own restrictions; calling it explicitly first is harmless and
> makes startup ordering explicit.

---

## `rlimits`

```go
import "github.com/altessa-s/go-atlas/core/runtime/rlimits"
```

Installs Linux process resource limits via `setrlimit(2)`. Each option caps both the soft and hard limit to the configured value. Hard-limit
reductions are irreversible for unprivileged processes. Intended for one-shot startup-time hardening.

| Option / Symbol            | Resource     | Effect                                             |
|----------------------------|--------------|----------------------------------------------------|
| `Apply(opts ...Option)`    |              | Installs every configured rlimit; validates first  |
| `WithMemoryBytes(int64)`   | `RLIMIT_AS`  | Cap virtual address space                          |
| `WithMaxOpenFiles(int64)`  | `RLIMIT_NOFILE` | Cap open file descriptors                       |
| `WithMaxProcesses(int64)`  | `RLIMIT_NPROC`  | Cap processes per real UID                      |
| `WithMaxFileSizeBytes(int64)` | `RLIMIT_FSIZE` | Cap maximum size of any file the process writes |
| `WithDisableCoreDumps()`   | `RLIMIT_CORE` | Pin to 0 (no core dumps)                          |
| `ErrUnsupported`           | sentinel     | Non-Linux                                          |
| `ErrFailed`                | sentinel     | Wraps `setrlimit(2)` errno                         |
| `ErrInvalidOption`         | sentinel     | Negative limit value                               |

```go
err := rlimits.Apply(
    rlimits.WithMemoryBytes(512<<20),
    rlimits.WithMaxOpenFiles(4096),
    rlimits.WithMaxProcesses(256),
    rlimits.WithDisableCoreDumps(),
)
switch {
case err == nil:
    // Resource caps in place.
case errors.Is(err, rlimits.ErrUnsupported):
    log.Print("rlimits: not supported on this platform; continuing")
case errors.Is(err, rlimits.ErrInvalidOption):
    log.Fatalf("rlimits: bad config: %v", err)
case errors.Is(err, rlimits.ErrFailed):
    log.Fatalf("rlimits: kernel rejected setrlimit: %v", err)
}
```

> **Zero means "leave alone".** A zero value on a numeric option leaves
> the kernel default in place for that resource. The single exception is
> `WithDisableCoreDumps`, which is a boolean because pinning
> `RLIMIT_CORE` to zero is itself a meaningful operation distinct from
> "leave it alone".

> **Process-wide and shared.** Unlike `capabilities` or `landlock`,
> rlimits apply to the entire process: every goroutine, every child,
> every in-process database driver competes for the same pool. Pick caps
> that account for the worst legitimate spike, not the steady state.

---

## `capabilities`

```go
import "github.com/altessa-s/go-atlas/core/runtime/capabilities"
```

cgo-free interface to Linux capabilities. Wraps `capset(2)`, `capget(2)`, and `prctl(PR_CAPBSET_DROP)` / `prctl(PR_CAP_AMBIENT, ...)` directly, without
libcap. For the "drop everything except what I need at startup" pattern.

| Symbol                              | Kind     | Purpose                                          |
|-------------------------------------|----------|--------------------------------------------------|
| `Get() (Sets, error)`               | function | Snapshot every capability set                    |
| `DropAll() error`                   | function | Drop every capability from every set             |
| `DropAllExcept(keep ...Cap) error`  | function | Drop everything except `keep`                    |
| `Apply(s Sets) error`               | function | Install a `Sets` snapshot transactionally        |
| `BoundingDrop(c Cap) error`         | function | Irreversibly remove `c` from the bounding set    |
| `AmbientClearAll() error`           | function | Clear every ambient bit                          |
| `AmbientRaise(c Cap) error`         | function | Add `c` to the ambient set                       |
| `AmbientLower(c Cap) error`         | function | Remove `c` from the ambient set                  |
| `ParseName(name string) (Cap, err)` | function | Resolve `"CAP_*"` strings (case-insensitive)     |
| `Sets`                              | struct   | `{Effective, Permitted, Inheritable, Bounding, Ambient}` |
| `Cap`                               | type     | Strongly-typed capability bit                    |
| `SetKind`                           | type     | `Effective` / `Permitted` / `Inheritable` / `Bounding` / `Ambient` |
| `CAP_*`                             | constants | 41 constants from `CAP_CHOWN` (0) to `CAP_CHECKPOINT_RESTORE` |
| `ErrUnsupported`                    | sentinel | Non-Linux                                        |
| `ErrFailed`                         | sentinel | Wraps `capset` / `capget` / `prctl` errno        |
| `ErrInvalidOption`                  | sentinel | Bad capability name or impossible operation      |

```go
// Common pattern: bind a privileged port, then drop everything.
if err := capabilities.DropAllExcept(capabilities.CAP_NET_BIND_SERVICE); err != nil {
    log.Fatalf("capabilities: %v", err)
}

// Resolving capability names from YAML configuration:
c, err := capabilities.ParseName("CAP_NET_BIND_SERVICE")
if err != nil {
    log.Fatalf("config: %v", err)
}
_ = capabilities.DropAllExcept(c)
```

> **Per-thread scope.** `capset(2)` is per-thread and Linux exposes no
> way to apply a new capability set to every thread of a process from
> userspace. The Go runtime has already created peer threads by the time
> user code runs. For a real process-wide drop, configure capabilities
> externally (`systemd CapabilityBoundingSet=`, container `--cap-drop`,
> a C launcher that drops before `execve(2)`); use this package as a
> defense-in-depth helper rather than the primary mechanism.

> **Irreversible ceilings.** Bits dropped from the bounding set can
> never be re-acquired by the calling thread or any descendant. Likewise
> `DropAll` / `DropAllExcept` are one-way: once they return, the listed
> capabilities are gone for the lifetime of that thread.

---

## `seccomp`

```go
import "github.com/altessa-s/go-atlas/core/runtime/seccomp"
```

Installs a fixed seccomp-BPF denylist of ~22 syscalls that a Go process never legitimately calls. A defense-in-depth primitive that reduces the blast
radius of a compromised plugin or dependency, NOT a complete syscall sandbox.

| Symbol                       | Kind     | Purpose                                       |
|------------------------------|----------|-----------------------------------------------|
| `BlockDangerousSyscalls()`   | function | Install the denylist filter (TSYNC; all threads) |
| `ErrUnsupported`             | sentinel | Non-Linux, or Linux on a non-amd64/arm64 arch |
| `ErrFailed`                  | sentinel | Wraps `seccomp(2)` / `prctl(2)` errno         |

### Why a denylist (not an allowlist)

A curated allowlist for a Go process is an antipattern:

- The Go runtime calls many syscalls that change between Go and kernel
  versions (`clone` vs `clone3`, `pidfd_open`, `madvise`, `rseq`, `membarrier`, `getrandom`, `futex`, `epoll_pwait2`, …).
- An over-eager allowlist breaks the process under `SIGSYS` at random
  moments in production, usually not at startup.
- A safe allowlist must be generated from production traces per Go
  version × kernel version. That is infrastructure work, not a Go package concern.

For syscall-level filtering beyond this fixed denylist, configure seccomp at the container layer: Kubernetes `securityContext.seccompProfile`, Docker
`--security-opt seccomp=…`, or systemd `SystemCallFilter=`.

### What the denylist blocks

| Category                  | Syscalls                                                  |
|---------------------------|-----------------------------------------------------------|
| Filesystem manipulation   | `mount`, `umount2`, `pivot_root`, `chroot`, `swapon`, `swapoff` |
| Kernel module loading     | `init_module`, `finit_module`, `delete_module`            |
| Kernel reload             | `kexec_file_load`                                         |
| System control            | `reboot`                                                  |
| Debugging / inspection    | `ptrace`, `process_vm_readv`, `process_vm_writev`         |
| Namespace manipulation    | `unshare`, `setns`                                        |
| Keyring                   | `keyctl`, `add_key`, `request_key`                        |
| Exotic escalation vectors | `userfaultfd`, `perf_event_open`, `bpf`                   |

A blocked syscall returns `EPERM`. An architecture mismatch (e.g. an x32 syscall on an amd64 kernel) kills the process outright, because syscall numbers
differ across arches and a filter that trusts the wrong numbering is worse than no filter.

```go
err := seccomp.BlockDangerousSyscalls()
switch {
case err == nil:
    // Filter installed; mount, kexec, init_module, ptrace, bpf, ...
    // are all blocked from this point on.
case errors.Is(err, seccomp.ErrUnsupported):
    log.Print("seccomp: not supported on this platform; continuing")
case errors.Is(err, seccomp.ErrFailed):
    log.Fatalf("seccomp: kernel rejected filter: %v", err)
}
```

> **Sets `PR_SET_NO_NEW_PRIVS` for you.** `BlockDangerousSyscalls` calls
> `nonewprivs.Set` internally because `seccomp(2)` requires either NNP
> or `CAP_SYS_ADMIN`. The TSYNC flag then propagates the filter to every
> thread in the thread group, which matters for a Go process where
> goroutines run on multiple OS threads.

> **Irreversible.** Seccomp filters form a stack and can only be made
> more restrictive; there is no "remove filter" syscall. Call this once,
> at startup.

---

## `landlock`

```go
import "github.com/altessa-s/go-atlas/core/runtime/landlock"
```

Wraps the unprivileged Linux Landlock LSM filesystem sandbox. Restricts the calling process to a strict allowlist of paths without `CAP_SYS_ADMIN` or
root. Restrictions are irreversible per process lifetime.

| Symbol                              | Kind     | Purpose                                       |
|-------------------------------------|----------|-----------------------------------------------|
| `Apply(opts ...Option) error`       | function | Build and install the ruleset; sets NNP first |
| `Supported() bool`                  | function | Report whether the kernel supports Landlock ABI ≥ 1 |
| `ABIVersion() (int, error)`         | function | Highest Landlock ABI version the kernel supports |
| `WithReadPaths(paths ...string)`    | option   | Grant read+execute access on the listed paths |
| `WithReadWritePaths(paths ...string)` | option | Grant read+execute+write+truncate access      |
| `ErrUnsupported`                    | sentinel | Non-Linux, or Linux < 5.13                    |
| `ErrFailed`                         | sentinel | Wraps `landlock_*(2)` / `openat` errno        |
| `ErrInvalidOption`                  | sentinel | Empty or non-absolute path                    |

```go
err := landlock.Apply(
    landlock.WithReadPaths(
        "/etc/myservice",
        "/lib", "/lib64",
        "/usr/lib", "/usr/lib64",
    ),
    landlock.WithReadWritePaths("/var/log/myservice"),
)
switch {
case err == nil:
    // Filesystem ruleset in force.
case errors.Is(err, landlock.ErrUnsupported):
    log.Print("landlock: not supported; continuing unsandboxed")
case errors.Is(err, landlock.ErrInvalidOption):
    log.Fatalf("landlock: bad config: %v", err)
case errors.Is(err, landlock.ErrFailed):
    log.Fatalf("landlock: kernel rejected ruleset: %v", err)
}
```

> **Strict allowlist.** Nothing is added implicitly. The ruleset must
> include the dynamic loader, libc, every shared library the binary
> opens at runtime, every config file, every data directory. Forgetting
> `/lib64` or `/usr/lib` will cause the next `dlopen` to fail with
> `EACCES`. Inventory the binary's filesystem footprint with `strace -f
> -e openat` before locking it down.

> **Sets `PR_SET_NO_NEW_PRIVS` for you.** `Apply` first probes
> `ABIVersion`; if Landlock is unsupported, it returns `ErrUnsupported`
> and leaves the process state untouched (in particular, NNP is **not**
> set). Once support is confirmed, it sets `PR_SET_NO_NEW_PRIVS` (a
> kernel prerequisite) and proceeds.

> **Symlinks resolved before check.** The restriction applies to the
> resolved target, not the link itself. A symlink that points outside
> the allowlist is just an unreadable file.

> **Irreversible and per-task.** There is no "unsandbox" syscall.
> Successive `Apply` calls compose by intersection: the allowlist
> monotonically shrinks. The per-task scope caveat from the top of this
> document applies: peer Go-runtime threads created before `Apply`
> retain their original (unrestricted) Landlock domain. New tasks
> `clone(2)`d from a restricted task do inherit the parent's domain.

---

## Error handling

Every package exposes the same three sentinel errors (where applicable):

| Sentinel           | Meaning                                                          |
|--------------------|------------------------------------------------------------------|
| `ErrUnsupported`   | The platform cannot honor the request; degrade gracefully.       |
| `ErrInvalidOption` | Operator misconfiguration; fail fast with a config error.        |
| `ErrFailed`        | The kernel rejected a well-formed request; wraps the raw errno.  |

The wrap chain preserves the kernel `syscall.Errno`, so callers can use both `errors.Is(err, pkg.ErrFailed)` for sentinel matching and `errors.As(err,
&errno)` to recover the raw errno for branching logic. The canonical pattern is a `switch` on the three sentinels:

```go
switch {
case err == nil:
    // success
case errors.Is(err, pkg.ErrUnsupported):
    // platform can't honor this; continue without the protection
case errors.Is(err, pkg.ErrInvalidOption):
    // operator gave us bad config; fail fast
case errors.Is(err, pkg.ErrFailed):
    // kernel said no; abort startup
default:
    // unexpected wrap; log and abort
}
```

---

## What these primitives do NOT do

- **Not a container replacement.** A namespace-isolated PID/mount/network
  view is out of scope. Use a container runtime, systemd unit isolation (`PrivateTmp=`, `ProtectSystem=`, `PrivateNetwork=`), or a microVM.
- **Not a memory-safety boundary.** A malicious dependency already
  loaded in-process can read every byte of the host's address space, including secrets, TLS keys, and connection state. Seccomp and Landlock close
  specific escalation vectors (container escape, kernel-module loading, ptrace, bpf, writing to disk outside the allowlist), not the in-process
  code-execution vector.
- **Not a network sandbox.** None of these packages restrict outbound
  connections. Use Linux network namespaces, eBPF, an egress firewall, or an L7 proxy.
- **Not retroactive across goroutines.** The per-thread primitives only
  affect the calling thread; peer Go-runtime threads created before hardening retain their original state. For a real process-wide guarantee, harden
  externally (systemd, container runtime, C launcher) before `execve(2)` of the Go binary.

---

## See also


- [Plugins](../../plugins.md): the primary untrusted-code vector this
  hardening stack defends against. The security warning at the top of that document is the motivation for everything here.
