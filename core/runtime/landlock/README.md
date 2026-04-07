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
(e.g. `core/io/files.Walk`, `core/plugins.NewManager`, `data/probfilter.NewManager`).
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
    err := landlock.Apply(
        landlock.WithReadPaths(
            "/etc/myservice",    // read-only config
            "/usr/lib",          // shared libraries
            "/usr/lib64",        // shared libraries (multilib)
            "/lib",              // dynamic loader + libc
            "/lib64",            // dynamic loader + libc (multilib)
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
`PR_SET_NO_NEW_PRIVS` on the calling process. `Apply` sets `NO_NEW_PRIVS`
unconditionally before touching the Landlock syscalls, so an unprivileged
process can call `Apply` with no preamble. The `prctl` is idempotent when
already set and, like Landlock itself, irreversible for the rest of the
process lifetime — calling `Apply` commits the process to both
restrictions. If you specifically need to preserve SUID-exec capability
post-Landlock (a `CAP_SYS_ADMIN` scenario), drop down to the raw syscalls
in `golang.org/x/sys/unix` rather than using this package.

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
| Plugin manager (see `core/plugins/sandbox.go`) | Plugins can't escape their allowlisted filesystem view |
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
- **Process-wide and irreversible.** No "unsandbox" syscall. Misconfigured
  limits require a process restart to clear.
- **Defense in depth, not isolation.** Landlock restricts filesystem
  access only. It does nothing about network, signals, process creation,
  or memory — a malicious binary already loaded in-process can still read
  and corrupt host memory.
- **Symlinks are resolved before the check.** A symlink pointing outside
  the allowlist is rejected.
- **Strict allowlist.** Nothing is auto-added. The caller lists every
  path the process needs, including the dynamic loader and libc.
- **`NO_NEW_PRIVS` may be set even on error.** `Apply` sets
  `PR_SET_NO_NEW_PRIVS` as its first step, *before* ABI detection and
  ruleset construction. If a later step fails (e.g. a listed path does not
  exist and `add_rule` returns ENOENT), `Apply` returns an `ErrFailed`
  with the `NO_NEW_PRIVS` bit still in effect. Operators should treat any
  invocation of `Apply` — successful or not — as a commitment to
  `NO_NEW_PRIVS` for the lifetime of the process.

## Testing

Because `Apply` is irreversible and process-wide, a unit test that
actually calls it would restrict every subsequent test in the same binary
(and many would fail). The test suite therefore only exercises the
validation layer (`validateOptions`) and the option accumulation behavior.
For real syscall coverage, run your integration tests in a dedicated
subprocess or container.

## See also

- [Kernel documentation](https://docs.kernel.org/userspace-api/landlock.html)
- [LWN article](https://lwn.net/Articles/859908/) (announcement, Landlock v1)
- `core/plugins/sandbox.go` in this repository — a real-world consumer that
  adds plugin-specific conveniences (auto-include plugin dir, system libs)
  on top of the raw `landlock.Apply` call.
