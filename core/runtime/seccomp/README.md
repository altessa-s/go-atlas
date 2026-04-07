# seccomp

```go
import "github.com/altessa-s/go-atlas/core/runtime/seccomp"
```

A minimal, cgo-free Go wrapper around the Linux seccomp-BPF
subsystem that installs a **fixed denylist** of ~22 syscalls a Go
process never legitimately calls. One function, no configuration, no
allowlist API — intentionally.

## API

| Symbol | Purpose |
|---|---|
| `BlockDangerousSyscalls()` | Installs the denylist filter; **irreversible** for the process |
| `ErrUnsupported` | Platform doesn't support seccomp-BPF (non-Linux, or Linux on an arch other than amd64/arm64) |
| `ErrFailed` | The seccomp/prctl syscall failed (sentinel wrapping the kernel errno via `%w`) |

## Quick start

```go
package main

import (
    "log"

    "github.com/altessa-s/go-atlas/core/runtime/seccomp"
)

func main() {
    if err := seccomp.BlockDangerousSyscalls(); err != nil {
        log.Fatalf("seccomp: %v", err)
    }
    run()
}
```

That's the entire usage surface. There is no `Block(syscalls ...int)`,
no `Allow(...)`, no profile file to load. The denylist is hard-coded
and audited — see "Why no allowlist" below.

## What it blocks

Every syscall in this list is a syscall the Go runtime has never
called and which a legitimate plugin host has no reason to invoke:

| Category | Syscalls |
|---|---|
| Filesystem manipulation | `mount`, `umount2`, `pivot_root`, `chroot`, `swapon`, `swapoff` |
| Kernel module loading | `init_module`, `finit_module`, `delete_module` |
| Kernel reload | `kexec_file_load` |
| System control | `reboot` |
| Debugging / memory inspection | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Namespace manipulation | `unshare`, `setns` |
| Keyring | `keyctl`, `add_key`, `request_key` |
| Exotic escalation vectors | `userfaultfd`, `perf_event_open`, `bpf` |

A blocked syscall returns `EPERM` to the caller. An architecture
mismatch (e.g. an x32 syscall on an amd64 kernel) kills the process
outright, because syscall numbers differ across arches and a filter
that trusts the wrong numbering is worse than no filter at all.

## Why no allowlist

A curated **allowlist** of syscalls for a Go-runtime process is an
anti-pattern, and this package refuses to support one. The reasons:

1. **The Go runtime calls many syscalls that change between Go
   versions and kernel versions.** `clone` ↔ `clone3`, `pidfd_open`,
   `madvise`, `rseq`, `membarrier`, `getrandom`, `futex`,
   `epoll_pwait2`, and more. An allowlist captured today breaks
   silently when you upgrade Go 1.25 → 1.26 or move from kernel 5.15
   → 6.6.

2. **Over-eager allowlists fail in production, not at startup.**
   When the GC, scheduler, or preemption machinery calls a
   non-whitelisted syscall, the process dies under `SIGSYS` — often
   under load, hours into the run. Debugging is miserable.

3. **Safe allowlists require production traces.** The only way to
   build a reliable allowlist is to run the workload under
   `strace -ff -c` (or a seccomp log-only mode) in staging, collect
   the union of observed syscalls, and pin that as the allowed set.
   That's infrastructure work, not a Go-package concern.

4. **Container-level seccomp is the right layer.** If you need more
   than this package's fixed denylist, configure seccomp at the
   container boundary:

   - **Kubernetes:** `spec.securityContext.seccompProfile` or a
     `RuntimeDefault` / `Localhost` profile.
   - **Docker:** `--security-opt seccomp=path/to/profile.json`.
   - **systemd:** `SystemCallFilter=` in the unit file.

   These layers let SRE edit the profile without rebuilding the
   binary, roll it out via the orchestrator, and share it across
   every process in the pod.

## When to use it

- **Plugin hosts** (see `core/plugins/sandbox.go`) where plugins
  loaded via `dlopen` would otherwise inherit the host's full
  syscall surface.
- **Long-running services** that have no reason to call `mount`,
  `ptrace`, `bpf`, or the kernel-module syscalls.
- **CLI tools processing untrusted input** (archive extractors,
  template renderers) that want to narrow the blast radius of a
  hypothetical RCE.
- **As one layer in the go-atlas hardening sequence:**
  `nonewprivs` → `rlimits` → `capabilities` → `seccomp` → `landlock`.

## Caveats

- **Linux 3.17+ on amd64 or arm64.** On non-Linux platforms,
  `BlockDangerousSyscalls` returns `ErrUnsupported`. On Linux
  architectures other than amd64 and arm64, ditto — the per-arch
  constants files only cover the two mainstream server arches, and
  supporting a new one requires verifying that every syscall in
  the denylist is present in `golang.org/x/sys/unix` for that arch.
- **Process-wide via TSYNC.** The filter applies to every thread in
  the thread group via `SECCOMP_FILTER_FLAG_TSYNC`, so it covers
  Go goroutines correctly regardless of which OS thread they run
  on. If TSYNC fails (a peer thread is in an uninterruptible
  syscall), the error includes the failing TID for debugging.
- **Irreversible.** Seccomp filters form a stack and can only be
  made more restrictive; there is no "remove filter" syscall. Like
  every primitive in the `core/runtime` hardening family.
- **`PR_SET_NO_NEW_PRIVS` is set automatically.** seccomp(2)
  requires either that prctl or `CAP_SYS_ADMIN`, and this package
  delegates to `core/runtime/nonewprivs.Set()` internally so
  callers never need to invoke `prctl` themselves.
- **Defense in depth, not isolation.** A malicious dependency
  already loaded in-process can still read every byte of memory
  in the host's address space. Seccomp closes specific escalation
  vectors (container escape, kernel-module loading, ptrace, bpf,
  etc.), not the ambient code-execution vector.

## Smoke test (manual)

Because `BlockDangerousSyscalls` is irreversible and process-wide,
we do not unit-test the actual syscall installation. For real
syscall coverage, build this ~30-line harness and run it on any
Linux 3.17+ host:

```go
package main

import (
    "fmt"
    "log"
    "os"

    "golang.org/x/sys/unix"

    "github.com/altessa-s/go-atlas/core/runtime/seccomp"
)

func main() {
    if err := seccomp.BlockDangerousSyscalls(); err != nil {
        log.Fatalf("BlockDangerousSyscalls: %v", err)
    }

    // A normal syscall must still work.
    f, err := os.Open("/etc/hosts")
    if err != nil {
        log.Fatalf("open /etc/hosts: %v (expected success)", err)
    }
    _ = f.Close()
    fmt.Println("open /etc/hosts: ok")

    // mount must return EPERM, not succeed, and not crash.
    err = unix.Mount("none", "/mnt", "tmpfs", 0, "")
    switch {
    case err == nil:
        log.Fatal("mount: got nil, want EPERM")
    case err == unix.EPERM:
        fmt.Println("mount: EPERM (expected)")
    default:
        log.Fatalf("mount: got %v, want EPERM", err)
    }

    fmt.Println("ok")
}
```

Build and run with `go run smoke.go`. Output should be:

```
open /etc/hosts: ok
mount: EPERM (expected)
ok
```

## See also

- [`seccomp(2)` man page](https://man7.org/linux/man-pages/man2/seccomp.2.html)
- [Kernel seccomp documentation](https://docs.kernel.org/userspace-api/seccomp_filter.html)
- Docker's [default seccomp profile](https://github.com/moby/moby/blob/master/profiles/seccomp/default.json)
  — a reference for container-level seccomp configuration.
- `core/runtime/nonewprivs` — `PR_SET_NO_NEW_PRIVS` primitive, the
  prerequisite for this package (set automatically).
- `core/runtime/rlimits` — resource caps, second step in the
  hardening sequence.
- `core/runtime/capabilities` — Linux capability dropping, third
  step.
- `core/runtime/landlock` — filesystem allowlist, final step.
- `core/plugins/sandbox.go` — real-world consumer of the other
  four hardening primitives. Seccomp integration is an optional
  follow-up when a concrete plugin-sandbox use case appears.
